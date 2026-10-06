package access

import "time"

// Entry is an Audit log entry (ADR 0033): when it happened, what, who did it
// and whom it concerns, each as named then, the browser involved, if any, and
// what else the event tells. It never holds an address.
type Entry struct {
	Time    time.Time      `json:"time"`
	Event   Event          `json:"event"`
	Actor   Party          `json:"actor"`
	Subject Party          `json:"subject"`
	Browser string         `json:"browser,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// Party is who acts or is acted upon in an Entry: an identity, the host,
// Oiko itself or unknown, with the Name it had then.
type Party struct {
	Kind Kind   `json:"kind"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Anonymous reports whether e is a refusal where no identity was found: the
// Audit log writes only so many of those (ADR 0034).
func (e Entry) Anonymous() bool { return e.Subject.Kind == UnknownKind }

// Event is what an Entry records.
type Event string

// Refusals; the "count" of an hourly Entry tells how many more of its kind
// came in that hour, past the budget of anonymous ones (ADR 0034).
const (
	SetupRefused   Event = "setup-refused"   // a Setup link that is not, or no longer, valid
	LinkRefused    Event = "link-refused"    // a Sign-in link that is not, or no longer, valid
	PasskeyRefused Event = "passkey-refused" // an unknown Passkey, a counter that went backwards, an answer that does not verify
	TokenRefused   Event = "token-refused"   // a Token that is not, or no longer, valid
	StepUpRefused  Event = "step-up-refused" // a Passkey that did not confirm a step-up
)

// Changes to access.
const (
	SignedIn       Event = "signed-in"        // a Session started; "method" tells how: setup, passkey or link (its "by" if not their own)
	SessionEnded   Event = "session-ended"    // "reason": signed out, expired, access ended, or removed with its Person
	PersonCreated  Event = "person-created"   // "level"
	PersonRenamed  Event = "person-renamed"   // "from" the previous Name
	PersonRemoved  Event = "person-removed"   //
	LevelChanged   Event = "level-changed"    // a Person's or a Program's, "from" and "to"
	EndDateChanged Event = "end-date-changed" // a Guest's, set, changed or removed: "from" and "to", null for none
	EndDateReached Event = "end-date-reached" // dated when it came: the Guest's Sessions ended
	LinkCreated    Event = "link-created"     // "expires"
	LinkRevoked    Event = "link-revoked"     //
	LinkExpired    Event = "link-expired"     // unused, dated when it expired
	PasskeyAdded   Event = "passkey-added"    // "provider", if known
	PasskeyRemoved Event = "passkey-removed"  // "provider", if known
	ProgramCreated Event = "program-created"  // "level"
	ProgramRenamed Event = "program-renamed"  // "from" the previous Name
	ProgramRemoved Event = "program-removed"  //
	TokenGenerated Event = "token-generated"  //
	TokenRevoked   Event = "token-revoked"    //
)

// Parties that are no identity.
var (
	nobody = Party{Kind: UnknownKind}
	oiko   = Party{Kind: OikoKind} // an expiry
)

// party is who id is, as named now.
func party(id Identity) Party { return Party{id.Kind, id.ID, id.Name} }

// record writes e to the Audit log, at now unless it has a time. Callers hold
// s.mu: entries keep the order of what they record.
func (s *Store) record(e Entry) {
	if e.Time.IsZero() {
		e.Time = s.now()
	}
	s.audit(e)
}
