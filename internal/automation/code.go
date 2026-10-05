package automation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	starmath "go.starlark.net/lib/math"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/llehouerou/oiko/internal/home"
)

// A Code Step runs def run(trigger, state) in Starlark (ADR 0005). It sees
// Starlark core, the math and time modules, and three names of its own:
//
//   - value(alias, capability): the Value of a bound target as of the
//     triggering Update, or None when unknown;
//   - command(alias, transition=None, **capabilities): a Command to a bound
//     target, in seconds;
//   - now: the Run's time, which time.now() returns too.
//
// It returns a handle name, a list of them, or None. A call is all or
// nothing: its Commands are issued, its state kept and its handles fired only
// if it returns cleanly within maxSteps. Otherwise the error goes into the
// Trace, and the rest of the Run goes on.
const (
	maxSteps = 100_000  // Starlark steps per call, or at load
	maxState = 16 << 10 // bytes of a Code Step's state, in JSON
	maxPrint = 1 << 10  // bytes a call prints into the Trace
	maxDepth = 64       // nesting of lists and dicts in a state or a Command
)

// codeStep is a Code Step: its script, and the state its calls keep.
// Params: {source, outputs, bindings}. Handles: run → its declared outputs.
type codeStep struct {
	script
	state []byte // in JSON; nil when empty
}

// script is a Code Step's source, compiled and run once at load: its frozen
// run function, the names it sees besides its own, its aliases and the
// targets bound to them, in order, and the outputs it declares.
type script struct {
	run         *starlark.Function
	predeclared starlark.StringDict // "now" is set anew for each call
	aliases     []string
	targets     []home.Target
	outputs     []string
}

type codeParams struct {
	Source   string                 `json:"source"`
	Outputs  []string               `json:"outputs"`
	Bindings map[string]home.Target `json:"bindings"`
}

func parseCode(s *step, p codeParams, _ *Place) error {
	for i, o := range p.Outputs {
		if o == "" || slices.Contains(p.Outputs[:i], o) {
			return fmt.Errorf("output %q: empty or duplicate", o)
		}
	}
	sc := script{aliases: slices.Sorted(maps.Keys(p.Bindings)), outputs: p.Outputs}
	for _, alias := range sc.aliases {
		t := p.Bindings[alias]
		if t.IsZero() {
			return fmt.Errorf("alias %q is bound to no target", alias)
		}
		sc.targets = append(sc.targets, t)
	}
	sc.predeclared = starlark.StringDict{"value": starlark.NewBuiltin("value", valueBuiltin),
		"command": starlark.NewBuiltin("command", commandBuiltin),
		"math":    starmath.Module, "time": startime.Module, "now": starlark.None} // no Run at load
	f, prog, err := starlark.SourceProgramOptions(&syntax.FileOptions{}, "source", p.Source, sc.predeclared.Has)
	if err != nil {
		return err
	}
	if err := sc.checkAliases(f); err != nil {
		return err
	}
	thread := &starlark.Thread{Name: "load", Print: func(*starlark.Thread, string) {}} // no load(): Load is nil
	thread.SetMaxExecutionSteps(maxSteps)
	startime.SetNow(thread, func() (time.Time, error) { return time.Time{}, errors.New("no time.now() at load") })
	globals, err := prog.Init(thread, sc.predeclared)
	if err != nil {
		return err
	}
	globals.Freeze()
	run, ok := globals["run"].(*starlark.Function)
	if !ok || run.NumParams() != 2 {
		return errors.New("no def run(trigger, state)")
	}
	sc.run = run
	s.kind, s.targets, s.outputs = &codeStep{script: sc}, sc.targets, sc.outputs
	return nil
}

// reached calls run: all or nothing, with what it printed in the Trace.
func (c *codeStep) reached(x visit, _ int) {
	before := c.state
	rn := x.e.runner
	outs, err := rn.call(c, &x.e.trace.Trigger, x.now())
	r := x.record()
	if len(rn.print) > 0 {
		r.Print = string(rn.print)
	}
	if err != nil {
		r.Error = errorText(err)
		return
	}
	if !bytes.Equal(before, c.state) {
		x.e.touch()
	}
	for _, b := range rn.commands {
		x.issue(b.target, b.req)
	}
	for _, out := range outs {
		x.fire(out)
	}
}

func (c *codeStep) save(st *StepState)              { st.State = c.state }
func (c *codeStep) restore(_ *Engine, st StepState) { c.state = st.State }
func (c *codeStep) forget()                         { c.state = nil }

// checkAliases checks the aliases f passes to value and command as literals;
// others are checked as it runs.
func (sc *script) checkAliases(f *syntax.File) (err error) {
	syntax.Walk(f, func(n syntax.Node) bool {
		c, ok := n.(*syntax.CallExpr)
		if !ok || len(c.Args) == 0 {
			return err == nil
		}
		fn, isIdent := c.Fn.(*syntax.Ident)
		lit, isLit := c.Args[0].(*syntax.Literal)
		if isIdent && (fn.Name == "value" || fn.Name == "command") && isLit && lit.Token == syntax.STRING &&
			!slices.Contains(sc.aliases, lit.Value.(string)) {
			err = fmt.Errorf("unknown alias %q", lit.Value)
		}
		return err == nil
	})
	return err
}

// runner runs Code Steps one call at a time, on one Starlark thread. The
// builtins act on the call under way.
type runner struct {
	thread   *starlark.Thread
	args     starlark.Tuple // run's, reused
	trigger  triggerValue
	values   map[home.Ref]home.Value // the mirror
	code     *codeStep
	now      time.Time
	commands []buffered
	print    []byte
}

// buffered is a Command a call issues if it returns cleanly.
type buffered struct {
	target home.Target
	req    home.Request
}

func newRunner(values map[home.Ref]home.Value) *runner {
	r := &runner{thread: &starlark.Thread{Name: "code"}, args: make(starlark.Tuple, 2), values: values}
	r.thread.Print = r.printed
	r.thread.SetLocal("runner", r)
	startime.SetNow(r.thread, func() (time.Time, error) { return r.now, nil })
	return r
}

func runnerOf(t *starlark.Thread) *runner { return t.Local("runner").(*runner) }

// call runs Code Step c for tg at now. It returns the handles to fire, with
// c's state and r.commands updated, or an error with its state untouched.
// Either way, r.print holds what it printed.
func (r *runner) call(c *codeStep, tg *home.Trigger, now time.Time) ([]int, error) {
	r.code, r.now, r.commands, r.print = c, now, r.commands[:0], r.print[:0]
	state, err := decodeState(c.state) // a copy
	if err != nil {
		return nil, err
	}
	r.trigger = triggerValue{tg, &c.script}
	r.args[0], r.args[1] = &r.trigger, state
	c.predeclared["now"] = startime.Time(now) // run looks it up as it runs
	r.thread.SetMaxExecutionSteps(r.thread.ExecutionSteps() + maxSteps)
	res, err := starlark.Call(r.thread, c.run, r.args, nil)
	r.thread.Uncancel()
	r.args[0], r.args[1] = nil, nil
	if err != nil {
		return nil, err
	}
	outs, err := c.handles(res)
	if err != nil {
		return nil, err
	}
	encoded, err := encodeState(state)
	if err != nil {
		return nil, err
	}
	c.state = encoded
	return outs, nil
}

func (r *runner) printed(_ *starlark.Thread, msg string) {
	if len(r.print) >= maxPrint {
		return
	}
	r.print = append(append(r.print, msg...), '\n')
	if len(r.print) > maxPrint {
		r.print = append(r.print[:maxPrint], "…"...)
	}
}

func (r *runner) target(alias string) (home.Target, error) {
	i := slices.Index(r.code.aliases, alias)
	if i < 0 {
		return home.Target{}, fmt.Errorf("unknown alias %q", alias)
	}
	return r.code.targets[i], nil
}

// errorText is err as a Trace shows it: with its backtrace, if run raised it.
func errorText(err error) string {
	if ee, ok := errors.AsType[*starlark.EvalError](err); ok {
		return ee.Backtrace()
	}
	return err.Error()
}

// handles returns the outputs named by what run returned.
func (sc *script) handles(res starlark.Value) ([]int, error) {
	if res == starlark.None {
		return nil, nil
	}
	l, ok := res.(*starlark.List)
	if !ok {
		out, err := sc.handle(res)
		return []int{out}, err
	}
	outs := make([]int, l.Len())
	for i := range outs {
		var err error
		if outs[i], err = sc.handle(l.Index(i)); err != nil {
			return nil, err
		}
	}
	return outs, nil
}

func (sc *script) handle(v starlark.Value) (int, error) {
	name, ok := v.(starlark.String)
	if !ok {
		return 0, fmt.Errorf("run returned a %s, not a handle name, a list of them or None", v.Type())
	}
	out := slices.Index(sc.outputs, string(name))
	if out < 0 {
		return 0, fmt.Errorf("undeclared handle %q", string(name))
	}
	return out, nil
}

// valueBuiltin is value(alias, capability).
func valueBuiltin(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var alias, capability string
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 2, &alias, &capability); err != nil {
		return nil, err
	}
	r := runnerOf(t)
	target, err := r.target(alias)
	if err != nil {
		return nil, err
	}
	v, ok := r.values[target.Ref(capability)]
	if !ok {
		return starlark.None, nil
	}
	return toStarlark(v.Data)
}

// commandBuiltin is command(alias, transition=None, **capabilities).
func commandBuiltin(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, fmt.Errorf("%s: want an alias, and a transition", b.Name())
	}
	alias, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("%s: the alias is a %s, not a string", b.Name(), args[0].Type())
	}
	r := runnerOf(t)
	target, err := r.target(alias)
	if err != nil {
		return nil, err
	}
	var transition starlark.Value = starlark.None
	if len(args) == 2 {
		transition = args[1]
	}
	values := make(map[string]any, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "transition" {
			if len(args) == 2 {
				return nil, fmt.Errorf("%s: transition given twice", b.Name())
			}
			transition = kv[1]
			continue
		}
		if values[key], err = toGo(kv[1], false, 0); err != nil {
			return nil, fmt.Errorf("%s: %s: %v", b.Name(), key, err)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s: no capabilities", b.Name())
	}
	req := home.Request{Values: values}
	if transition != starlark.None {
		s, ok := starlark.AsFloat(transition)
		if !ok || !(s >= 0) || math.IsInf(s, 0) {
			return nil, fmt.Errorf("%s: the transition must be 0 or more seconds, not %s", b.Name(), transition)
		}
		req.Transition = seconds(s)
	}
	r.commands = append(r.commands, buffered{target, req})
	return starlark.None, nil
}

// triggerValue is a Run's trigger as a Code Step sees it: a read-only struct.
// Its key and source name the triggering target, or are None without one.
type triggerValue struct {
	tg *home.Trigger
	sc *script
}

var triggerAttrs = []string{"capability", "event", "key", "kind", "source", "time", "value"}

func (*triggerValue) String() string        { return "trigger" }
func (*triggerValue) Type() string          { return "trigger" }
func (*triggerValue) Freeze()               {}
func (*triggerValue) Truth() starlark.Bool  { return starlark.True }
func (*triggerValue) Hash() (uint32, error) { return 0, errors.New("unhashable: trigger") }
func (*triggerValue) AttrNames() []string   { return triggerAttrs }

func (t *triggerValue) Attr(name string) (starlark.Value, error) {
	tg := t.tg
	switch name {
	case "kind":
		return starlark.String(strings.TrimSuffix(tg.Kind, "Trigger")), nil
	case "source":
		if i := slices.Index(t.sc.targets, tg.Target); i >= 0 {
			return starlark.String(t.sc.aliases[i]), nil
		}
		return starlark.None, nil
	case "key":
		if tg.Target.IsZero() {
			return starlark.None, nil
		}
		return starlark.String(tg.Target.Key()), nil
	case "capability":
		if tg.Capability == "" {
			return starlark.None, nil
		}
		return starlark.String(tg.Capability), nil
	case "value":
		if tg.Kind == eventTrigger {
			return starlark.None, nil
		}
		return toStarlark(tg.Value)
	case "event":
		if tg.Kind != eventTrigger {
			return starlark.None, nil
		}
		return toStarlark(tg.Value)
	case "time":
		return startime.Time(tg.Time), nil
	}
	return nil, nil
}

// decodeState returns a fresh dict holding state data.
func decodeState(data []byte) (*starlark.Dict, error) {
	if data == nil {
		return starlark.NewDict(0), nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	s, err := toStarlark(v)
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	d, ok := s.(*starlark.Dict)
	if !ok {
		return nil, errors.New("state: not an object")
	}
	return d, nil
}

// encodeState returns state d in JSON, nil if empty, or why it can't be kept.
func encodeState(d *starlark.Dict) ([]byte, error) {
	if d.Len() == 0 {
		return nil, nil
	}
	v, err := toGo(d, true, 0)
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}
	if len(data) > maxState {
		return nil, fmt.Errorf("state: %d bytes in JSON, over %d KiB", len(data), maxState>>10)
	}
	return data, nil
}

// toStarlark converts a Value's data, or JSON decoded with UseNumber.
func toStarlark(v any) (starlark.Value, error) {
	switch v := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(v), nil
	case float64:
		return starlark.Float(v), nil
	case string:
		return starlark.String(v), nil
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			f, err := v.Float64()
			return starlark.Float(f), err
		}
		if i, err := v.Int64(); err == nil {
			return starlark.MakeInt64(i), nil
		}
		b, ok := new(big.Int).SetString(string(v), 10)
		if !ok {
			return nil, fmt.Errorf("bad number %s", v)
		}
		return starlark.MakeBigInt(b), nil
	case []any:
		l := make([]starlark.Value, len(v))
		for i, e := range v {
			var err error
			if l[i], err = toStarlark(e); err != nil {
				return nil, err
			}
		}
		return starlark.NewList(l), nil
	case map[string]any:
		d := starlark.NewDict(len(v))
		for _, k := range slices.Sorted(maps.Keys(v)) {
			e, err := toStarlark(v[k])
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(starlark.String(k), e); err != nil {
				return nil, err
			}
		}
		return d, nil
	}
	return nil, fmt.Errorf("unsupported %T", v)
}

// toGo converts v to JSON-able Go: exactly for a state, where json.Numbers
// keep ints and floats apart, or as a Command's values, whose numbers are
// float64.
func toGo(v starlark.Value, exact bool, depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("nested too deep")
	}
	switch v := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.String:
		if !utf8.ValidString(string(v)) {
			return nil, fmt.Errorf("%q is not valid UTF-8", string(v))
		}
		return string(v), nil
	case starlark.Int:
		if exact {
			return json.Number(v.String()), nil
		}
		return float64(v.Float()), nil
	case starlark.Float:
		f := float64(v)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return nil, fmt.Errorf("%v is not JSON-able", v)
		}
		if !exact {
			return f, nil
		}
		s := strconv.FormatFloat(f, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return json.Number(s), nil
	case *starlark.List:
		l := make([]any, 0, v.Len())
		for e := range v.Elements() {
			g, err := toGo(e, exact, depth+1)
			if err != nil {
				return nil, err
			}
			l = append(l, g)
		}
		return l, nil
	case *starlark.Dict:
		m := make(map[string]any, v.Len())
		for k, e := range v.Entries() {
			key, ok := k.(starlark.String)
			if !ok {
				return nil, fmt.Errorf("key %s is not a string", k)
			}
			g, err := toGo(e, exact, depth+1)
			if err != nil {
				return nil, err
			}
			m[string(key)] = g
		}
		return m, nil
	}
	return nil, fmt.Errorf("a %s is not JSON-able", v.Type())
}
