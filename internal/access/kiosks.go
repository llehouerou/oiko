package access

import (
	"container/list"
	"crypto/rand"
	"fmt"
	"slices"
	"time"
	"uuid"

	"github.com/llehouerou/oiko/bridge/store"
	"github.com/llehouerou/oiko/internal/home"
)

// KiosksFormat is kiosks.json's; format 1.
var KiosksFormat store.Format

// Pairing requests in memory (ADR 0029, 0034).
const (
	pairingLife = 10 * time.Minute
	maxPairings = 10_000
)

// Kiosk is a shared screen signed in as itself (ADR 0029): a Guest or a
// Member, the Admin Person who paired it last, by id, when, and when it was
// last used.
type Kiosk struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Level    Level     `json:"level"`
	Pairer   string    `json:"pairer"`
	Paired   time.Time `json:"paired"`
	LastUse  time.Time `json:"lastUse,omitzero"`
	SignedIn bool      `json:"-"` // whether it holds a Session, as Kiosks tells; never stored
}

// Pairing is a pairing request as the Admin approving it sees it: when the
// screen made it, and in what browser.
type Pairing struct {
	Created time.Time
	Browser string
}

// pairing is a pairing request in memory: the hashes of its approval and
// claim secrets, since when it lasts (made or approved), and once approved,
// the Kiosk it pairs and the Admin Person who approved it.
type pairing struct {
	Pairing
	approval, claim string
	since           time.Time
	kiosk, by       string
}

// pairings are the requests in memory, oldest first in order.
type pairings struct {
	order               list.List // of *pairing
	byApproval, byClaim map[string]*list.Element
}

// add keeps p, dropping first those that expired and, at the cap, the
// oldest.
func (q *pairings) add(p *pairing) {
	for e := q.order.Front(); e != nil; e = q.order.Front() {
		if q.order.Len() < maxPairings && p.since.Sub(e.Value.(*pairing).since) < pairingLife {
			break
		}
		q.remove(e)
	}
	e := q.order.PushBack(p)
	q.byApproval[p.approval], q.byClaim[p.claim] = e, e
}

func (q *pairings) remove(e *list.Element) {
	p := q.order.Remove(e).(*pairing)
	delete(q.byApproval, p.approval)
	delete(q.byClaim, p.claim)
}

// find is the request that index holds for secret; nil if none lasts at now.
func (q *pairings) find(index map[string]*list.Element, secret string, now time.Time) *list.Element {
	e := index[hash(secret)]
	if e != nil && now.Sub(e.Value.(*pairing).since) >= pairingLife {
		q.remove(e)
		return nil
	}
	return e
}

// errPairingEnded refuses a pairing request that expired, was already
// approved or claimed, or never existed.
var errPairingEnded = fmt.Errorf("%w: this Kiosk pairing request expired, was refused or was already used; the screen shows a new code", ErrRefused)

// validKiosk checks a Kiosk's Name and Access level, never Admin (ADR
// 0023), answering the Name trimmed.
func validKiosk(name string, level Level) (string, error) {
	if level != Guest && level != Member {
		return "", fmt.Errorf("%w: access level %q: a Kiosk is a guest or a member", home.ErrInvalid, level)
	}
	return home.ValidName(name)
}

// RequestPairing makes a pairing request from a screen in browser, and
// answers its approval secret, for its QR code, and its claim secret, which
// only the screen holds (ADR 0029).
func (s *Store) RequestPairing(browser string) (approval, claim string) {
	approval, claim = rand.Text(), rand.Text()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	s.pairings.add(&pairing{Pairing: Pairing{now, browser}, approval: hash(approval), claim: hash(claim), since: now})
	return approval, claim
}

// PairingRequest answers the pairing request of approval to an Admin Person
// about to approve it.
func (s *Store) PairingRequest(by Identity, approval string) (Pairing, error) {
	if err := mayManage(by, false); err != nil {
		return Pairing{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.approvable(by, approval)
	if err != nil {
		return Pairing{}, err
	}
	return e.Value.(*pairing).Pairing, nil
}

// RefusePairing refuses the pairing request of approval, for an Admin
// Person: the screen that made it is refused, and shows a new code.
func (s *Store) RefusePairing(by Identity, approval string) error {
	if err := mayManage(by, false); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.approvable(by, approval)
	if err != nil {
		return err
	}
	p := e.Value.(*pairing)
	s.pairings.remove(e)
	s.record(Entry{Event: PairingRefused, Actor: party(by), Subject: nobody, Browser: p.Browser, Detail: map[string]any{"reason": "refused"}})
	return nil
}

// approvable is the pairing request of approval, not yet approved; refused,
// and recorded, if none lasts. Callers hold s.mu.
func (s *Store) approvable(by Identity, approval string) (*list.Element, error) {
	e := s.pairings.find(s.pairings.byApproval, approval, s.now())
	if e == nil {
		s.record(Entry{Event: PairingRefused, Actor: party(by), Subject: nobody, Detail: map[string]any{"reason": "expired"}})
		return nil, errPairingEnded
	}
	return e, nil
}

// ApprovePairing approves the pairing request of approval, for a fresh Admin
// Person: it pairs Kiosk kiosk, ending its Session, or a new Kiosk named
// name at level when kiosk is "", and answers the Kiosk's id. The screen
// then claims the Kiosk's Session.
func (s *Store) ApprovePairing(by Identity, approval, kiosk, name string, level Level) (string, error) {
	if err := mayManage(by, true); err != nil {
		return "", err
	}
	if kiosk == "" {
		var err error
		if name, err = validKiosk(name, level); err != nil {
			return "", err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.approvable(by, approval)
	if err != nil {
		return "", err
	}
	ks, i := slices.Clone(s.kiosks), s.kiosk(kiosk)
	if kiosk == "" {
		ks, i = append(ks, Kiosk{ID: uuid.NewV7().String(), Name: name, Level: level}), len(ks)
	} else if i < 0 {
		return "", fmt.Errorf("kiosk %q: %w", kiosk, home.ErrNotFound)
	}
	now := s.now()
	ks[i].Pairer, ks[i].Paired = by.ID, now
	if err := s.saveKiosks(ks); err != nil {
		return "", err
	}
	p := e.Value.(*pairing)
	delete(s.pairings.byApproval, p.approval)
	p.kiosk, p.by, p.since = ks[i].ID, by.ID, now
	s.pairings.order.MoveToBack(e)
	k := kioskParty(ks[i])
	if kiosk == "" {
		s.record(Entry{Event: KioskCreated, Actor: party(by), Subject: k, Detail: map[string]any{"level": level}})
	}
	s.record(Entry{Event: KioskPaired, Actor: party(by), Subject: k, Browser: p.Browser})
	return ks[i].ID, s.endKioskSession(ks[i].ID, party(by), "paired again")
}

// ClaimPairing answers the secret of the Kiosk's Session to the screen
// holding the claim secret of a request, once an Admin approved it; "" while
// none has. Its Session ends any other of the Kiosk.
func (s *Store) ClaimPairing(claim string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.pairings.find(s.pairings.byClaim, claim, s.now())
	if e == nil {
		s.record(Entry{Event: PairingRefused, Actor: nobody, Subject: nobody, Detail: map[string]any{"reason": "expired"}})
		return "", errPairingEnded
	}
	p := e.Value.(*pairing)
	if p.kiosk == "" {
		return "", nil
	}
	s.pairings.remove(e)
	i := s.kiosk(p.kiosk)
	if i < 0 { // removed since
		return "", errPairingEnded
	}
	if err := s.endKioskSession(p.kiosk, s.personParty(p.by), "paired again"); err != nil {
		return "", err
	}
	x := session{Kiosk: p.kiosk, Browser: p.Browser, Method: "pairing", By: p.by}
	secret, err := s.signIn(&x)
	if err != nil {
		return "", err
	}
	s.kiosks[i].LastUse = x.SignedIn
	s.used(&s.kioskUses, func() error { return s.saveKiosks(s.kiosks) })
	s.recordSignIn(kioskParty(s.kiosks[i]), x)
	return secret, nil
}

// Kiosks answers every Kiosk, oldest first, to an Admin Person.
func (s *Store) Kiosks(by Identity) ([]Kiosk, error) {
	if err := mayManage(by, false); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ks := slices.Clone(s.kiosks)
	for i := range ks {
		ks[i].SignedIn = s.kioskSession(ks[i].ID) != nil
	}
	return ks, nil
}

// EditKiosk renames Kiosk id and sets its Access level, for a fresh Admin
// Person; a new level ends its open event stream.
func (s *Store) EditKiosk(by Identity, id, name string, level Level) error {
	if err := mayManage(by, true); err != nil { // refused before told what is invalid
		return err
	}
	name, err := validKiosk(name, level)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.foundKiosk(id)
	if err != nil {
		return err
	}
	was, ks := s.kiosks[i], slices.Clone(s.kiosks)
	ks[i].Name, ks[i].Level = name, level
	if err := s.saveKiosks(ks); err != nil {
		return err
	}
	if name != was.Name {
		s.record(Entry{Event: KioskRenamed, Actor: party(by), Subject: kioskParty(ks[i]), Detail: map[string]any{"from": was.Name}})
	}
	if level != was.Level {
		s.record(Entry{Event: LevelChanged, Actor: party(by), Subject: kioskParty(ks[i]), Detail: map[string]any{"from": was.Level, "to": level}})
		if x := s.kioskSession(id); x != nil {
			s.finish(x.Hash)
		}
	}
	return nil
}

// SignOutKiosk ends the Session of Kiosk id, for an Admin Person, with no
// step-up: it only takes access away. The Kiosk stays, to be paired again.
func (s *Store) SignOutKiosk(by Identity, id string) error {
	if err := mayManage(by, false); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.foundKiosk(id); err != nil {
		return err
	}
	return s.endKioskSession(id, party(by), "revoked")
}

// RemoveKiosk removes Kiosk id, ending its Session, for an Admin Person,
// with no step-up.
func (s *Store) RemoveKiosk(by Identity, id string) error {
	if err := mayManage(by, false); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.foundKiosk(id)
	if err != nil {
		return err
	}
	k := s.kiosks[i]
	if err := s.saveKiosks(slices.Delete(slices.Clone(s.kiosks), i, i+1)); err != nil {
		return err
	}
	s.record(Entry{Event: KioskRemoved, Actor: party(by), Subject: kioskParty(k)})
	if x := s.kioskSession(id); x != nil {
		s.endSession(x, party(by), kioskParty(k), "removed", time.Time{})
	}
	// The Kiosk is gone: a Session left on disk without it is dropped on load.
	return s.saveSessions()
}

// endKioskSession ends the Session of Kiosk id, if it has one, as actor does
// for reason. Callers hold s.mu.
func (s *Store) endKioskSession(id string, actor Party, reason string) error {
	x := s.kioskSession(id)
	if x == nil {
		return nil
	}
	s.endSession(x, actor, s.holder(x), reason, time.Time{})
	return s.saveSessions()
}

// kioskSession is the Session of Kiosk id; nil if it has none. Callers hold
// s.mu.
func (s *Store) kioskSession(id string) *session {
	for _, x := range s.sessions {
		if x.Kiosk == id && !s.ended(x) {
			return x
		}
	}
	return nil
}

// kiosk is the index of Kiosk id; -1 if there is none. Callers hold s.mu.
func (s *Store) kiosk(id string) int {
	return slices.IndexFunc(s.kiosks, func(k Kiosk) bool { return k.ID == id })
}

// foundKiosk is the index of Kiosk id. Callers hold s.mu.
func (s *Store) foundKiosk(id string) (int, error) {
	i := s.kiosk(id)
	if i < 0 {
		return 0, fmt.Errorf("kiosk %q: %w", id, home.ErrNotFound)
	}
	return i, nil
}

// kioskParty is Kiosk k as the Audit log names it.
func kioskParty(k Kiosk) Party { return Party{KioskKind, k.ID, k.Name} }

// saveKiosks writes ks, last uses included, which are then the Kiosks.
// Callers hold s.mu.
func (s *Store) saveKiosks(ks []Kiosk) error {
	if err := store.Save(s.kiosksFile, KiosksFormat, ks); err != nil {
		return err
	}
	s.kiosks = ks
	s.kioskUses = pending{written: s.now()}
	return nil
}
