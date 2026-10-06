package access

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/llehouerou/oiko/internal/home"
)

// Passkey is a Person's WebAuthn credential as stored (ADR 0024, 0032): its
// id, in base64url, which the API names it by, its public key, the
// signature counter it last reported, the AAGUID naming its provider, and
// when it was created and last used.
type Passkey struct {
	ID             protocol.URLEncodedBase64 `json:"id"`
	PublicKey      []byte                    `json:"publicKey"`
	Counter        uint32                    `json:"counter"`
	BackupEligible bool                      `json:"backupEligible"` // synced by its provider; WebAuthn checks it never changes
	AAGUID         []byte                    `json:"aaguid,omitempty"`
	Created        time.Time                 `json:"created"`
	LastUse        time.Time                 `json:"lastUse,omitzero"`
}

// Ceremonies in memory (ADR 0032, 0034).
const (
	ceremonyLife  = 5 * time.Minute
	maxCeremonies = 10_000
)

// What a WebAuthn ceremony is for.
type purpose int

const (
	signingIn  purpose = iota // any Passkey of this Oiko answers
	steppingUp                // a Person's Passkey confirms it is them
	enrolling                 // a Person adds a Passkey
)

// ceremony is a WebAuthn ceremony begun and not yet finished.
type ceremony struct {
	purpose purpose
	data    webauthn.SessionData
	origin  string
	person  string // the Person stepping up or enrolling
	session string // the hash of the Session stepping up
	started time.Time
}

// ceremonies are those in progress by challenge, oldest first in order.
type ceremonies struct {
	order list.List // of *ceremony
	by    map[string]*list.Element
}

// add keeps c, dropping first those that expired and, at the cap, the
// oldest.
func (q *ceremonies) add(c *ceremony) {
	for e := q.order.Front(); e != nil; e = q.order.Front() {
		if q.order.Len() < maxCeremonies && c.started.Sub(e.Value.(*ceremony).started) < ceremonyLife {
			break
		}
		q.order.Remove(e)
		delete(q.by, e.Value.(*ceremony).data.Challenge)
	}
	q.by[c.data.Challenge] = q.order.PushBack(c)
}

// take ends the ceremony of challenge and answers it; nil if there is none
// that lasts at now.
func (q *ceremonies) take(challenge string, now time.Time) *ceremony {
	e := q.by[challenge]
	if e == nil {
		return nil
	}
	q.order.Remove(e)
	delete(q.by, challenge)
	if c := e.Value.(*ceremony); now.Sub(c.started) < ceremonyLife {
		return c
	}
	return nil
}

// relyingParty is Oiko as a WebAuthn Relying Party to a browser at origin,
// one sign-in accepts (ADR 0024): the origin's host is the RP ID, and only
// Passkeys that verify their user are accepted, attestation "none".
func relyingParty(origin string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return nil, fmt.Errorf("%w: origin %q", home.ErrInvalid, origin)
	}
	return webauthn.New(&webauthn.Config{
		RPID:                  u.Hostname(),
		RPDisplayName:         "Oiko",
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationRequired,
		},
	})
}

// user is Person p as WebAuthn sees them: their id is their user handle.
type user struct{ p *Person }

func (u user) WebAuthnID() []byte          { return []byte(u.p.ID) }
func (u user) WebAuthnName() string        { return u.p.Name }
func (u user) WebAuthnDisplayName() string { return u.p.Name }

func (u user) WebAuthnCredentials() []webauthn.Credential {
	cs := make([]webauthn.Credential, len(u.p.Passkeys))
	for i, p := range u.p.Passkeys {
		cs[i] = webauthn.Credential{
			ID:            p.ID,
			PublicKey:     p.PublicKey,
			Flags:         webauthn.CredentialFlags{BackupEligible: p.BackupEligible},
			Authenticator: webauthn.Authenticator{AAGUID: p.AAGUID, SignCount: p.Counter},
		}
	}
	return cs
}

// keep keeps ceremony c, just begun, whose WebAuthn data is data. Callers
// hold s.mu.
func (s *Store) keep(c ceremony, data *webauthn.SessionData) {
	c.data, c.started = *data, s.now()
	s.ceremonies.add(&c)
}

// ceremony ends the ceremony for purpose whose challenge clientData names,
// as finished at origin; refused if there is none. Callers hold s.mu.
func (s *Store) ceremony(clientData protocol.CollectedClientData, p purpose, origin string) (*ceremony, error) {
	c := s.ceremonies.take(clientData.Challenge, s.now())
	if c == nil || c.purpose != p || c.origin != origin {
		return nil, fmt.Errorf("%w: this Passkey request ended or was already used; try again", ErrRefused)
	}
	return c, nil
}

// passkeyRefused is why WebAuthn refused an answer.
func passkeyRefused(err error) error {
	var pe *protocol.Error
	if errors.As(err, &pe) && pe.Details != "" {
		return fmt.Errorf("%w: the Passkey was not accepted: %s", ErrRefused, pe.Details)
	}
	return fmt.Errorf("%w: the Passkey was not accepted: %v", ErrRefused, err)
}

// BeginSignIn starts a sign-in with a Passkey in a browser at origin, one
// where sign-in works: the options for its passkey sheet, which picks the
// account, any Passkey of this Oiko (ADR 0024).
func (s *Store) BeginSignIn(origin string) (*protocol.CredentialAssertion, error) {
	rp, err := relyingParty(origin)
	if err != nil {
		return nil, err
	}
	options, data, err := rp.BeginDiscoverableLogin()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keep(ceremony{purpose: signingIn, origin: origin}, data)
	return options, nil
}

// FinishSignIn checks the answer of a Passkey to a sign-in begun at origin,
// and answers the secret of a new Session for its Person in browser.
func (s *Store) FinishSignIn(origin string, response []byte, browser string) (string, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return "", fmt.Errorf("%w: %v", home.ErrInvalid, err)
	}
	rp, err := relyingParty(origin)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.ceremony(parsed.Response.CollectedClientData, signingIn, origin)
	if err != nil {
		return "", err
	}
	who, unknown := -1, false
	_, credential, err := rp.ValidatePasskeyLogin(func(_, handle []byte) (webauthn.User, error) {
		if who = s.person(string(handle)); who < 0 {
			unknown = true
			return nil, errors.New("no such Person")
		}
		return user{&s.persons[who]}, nil
	}, c.data, parsed)
	switch {
	case unknown:
		err = fmt.Errorf("%w: this Passkey belongs to no one in this Oiko; ask an Admin for a Sign-in link", ErrRefused)
	case err != nil:
		err = passkeyRefused(err)
	case over(s.persons[who], s.now()):
		err = errAccessEnded
	default:
		err = s.signedWith(who, credential)
	}
	if err != nil {
		subject := nobody
		if who >= 0 {
			subject = personParty(s.persons[who])
		}
		s.record(Entry{Event: PasskeyRefused, Actor: nobody, Subject: subject, Browser: browser, Detail: map[string]any{"reason": err.Error()}})
		return "", err
	}
	p := s.persons[who]
	x := session{Person: p.ID, Browser: browser, Method: "passkey", Provider: Provider(credential.Authenticator.AAGUID)}
	secret, err := s.signIn(&x)
	if err == nil {
		s.recordSignIn(p, x)
	}
	return secret, err
}

// signedWith records a sign-in or a step-up with Person i's Passkey credential: its
// counter and last use, written at once (ADR 0032). A counter that did not
// go up, when either is not 0, is refused: the Passkey may have been copied.
// Callers hold s.mu.
func (s *Store) signedWith(i int, credential *webauthn.Credential) error {
	if credential.Authenticator.CloneWarning {
		return fmt.Errorf("%w: this Passkey's signature counter went backwards, so it may have been copied; use another Passkey, or ask for a Sign-in link", ErrRefused)
	}
	return s.changePasskeys(i, func(ks []Passkey) []Passkey {
		for j := range ks {
			if bytes.Equal(ks[j].ID, credential.ID) {
				ks[j].Counter, ks[j].LastUse = credential.Authenticator.SignCount, s.now()
			}
		}
		return ks
	})
}

// changePasskeys has change edit a copy of the Passkeys of Person i, and
// writes them. Callers hold s.mu.
func (s *Store) changePasskeys(i int, change func([]Passkey) []Passkey) error {
	ps := slices.Clone(s.persons)
	ps[i].Passkeys = change(slices.Clone(ps[i].Passkeys))
	return s.savePersons(ps)
}

// BeginStepUp starts a step-up of the Session of secret, in a browser at
// origin: the options for the passkey sheet, which offers only its Person's
// Passkeys. A Person without one signs in by link again instead.
func (s *Store) BeginStepUp(secret, origin string) (*protocol.CredentialAssertion, error) {
	rp, err := relyingParty(origin)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	x, i, err := s.lasting(secret)
	if err != nil {
		return nil, err
	}
	if len(s.persons[i].Passkeys) == 0 {
		return nil, fmt.Errorf("%w: you have no Passkey to confirm it is you; sign in again with a new Sign-in link to do this", ErrRefused)
	}
	options, data, err := rp.BeginLogin(user{&s.persons[i]})
	if err != nil {
		return nil, err
	}
	s.keep(ceremony{purpose: steppingUp, origin: origin, person: x.Person, session: x.Hash}, data)
	return options, nil
}

// FinishStepUp checks the answer of a Passkey to the step-up begun at origin
// for the Session of secret, which is then fresh for step-up (ADR 0025).
func (s *Store) FinishStepUp(secret, origin string, response []byte) error {
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return fmt.Errorf("%w: %v", home.ErrInvalid, err)
	}
	rp, err := relyingParty(origin)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.ceremony(parsed.Response.CollectedClientData, steppingUp, origin)
	if err != nil {
		return err
	}
	x, i, err := s.lasting(secret)
	if err != nil {
		return err
	}
	if x.Hash != c.session {
		return fmt.Errorf("%w: this Passkey request was another Session's", ErrRefused)
	}
	credential, err := rp.ValidateLogin(user{&s.persons[i]}, c.data, parsed)
	if err != nil {
		err = passkeyRefused(err)
	} else {
		err = s.signedWith(i, credential)
	}
	if err != nil {
		p := personParty(s.persons[i])
		s.record(Entry{Event: StepUpRefused, Actor: p, Subject: p, Browser: x.Browser, Detail: map[string]any{"reason": err.Error()}})
		return err
	}
	x.Confirmed = s.now()
	return s.saveSessions()
}

// lasting is the Session of secret and the index of its Person; refused if
// it ended. Callers hold s.mu.
func (s *Store) lasting(secret string) (*session, int, error) {
	x := s.sessions[hash(secret)]
	if x == nil || s.ended(x) || s.person(x.Person) < 0 {
		return nil, 0, fmt.Errorf("%w: sign in first", ErrRefused)
	}
	return x, s.person(x.Person), nil
}

// self is the index of Person by, who manages their own Passkeys and Name,
// fresh for step-up when stepUp; refused for anyone else. Callers hold s.mu.
func (s *Store) self(by Identity, stepUp bool) (int, error) {
	i := s.person(by.ID)
	if by.Kind != PersonKind || i < 0 {
		return 0, fmt.Errorf("%w: only a signed-in Person has Passkeys and a Name of their own", ErrRefused)
	}
	if stepUp {
		return i, by.StepUp()
	}
	return i, nil
}

// Passkeys answers the Passkeys of Person person, oldest first, to them, or
// to a fresh Admin Person.
func (s *Store) Passkeys(by Identity, person string) ([]Passkey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.whose(by, person, false)
	if err != nil {
		return nil, err
	}
	return slices.Clone(s.persons[i].Passkeys), nil
}

// BeginPasskey starts adding a Passkey to Person by, fresh for step-up, in a
// browser at origin: the options for creating it, a discoverable credential
// that verifies its user, on a device or provider not holding one of theirs.
func (s *Store) BeginPasskey(by Identity, origin string) (*protocol.CredentialCreation, error) {
	rp, err := relyingParty(origin)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.self(by, true)
	if err != nil {
		return nil, err
	}
	u := user{&s.persons[i]}
	var held []protocol.CredentialDescriptor
	for _, c := range u.WebAuthnCredentials() {
		held = append(held, c.Descriptor())
	}
	options, data, err := rp.BeginRegistration(u, webauthn.WithExclusions(held))
	if err != nil {
		return nil, err
	}
	s.keep(ceremony{purpose: enrolling, origin: origin, person: by.ID}, data)
	return options, nil
}

// FinishPasskey adds to Person by, still fresh for step-up, the Passkey of
// response, created for the ceremony begun at origin, and answers its id.
func (s *Store) FinishPasskey(by Identity, origin string, response []byte) (string, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return "", fmt.Errorf("%w: %v", home.ErrInvalid, err)
	}
	rp, err := relyingParty(origin)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.self(by, true)
	if err != nil {
		return "", err
	}
	c, err := s.ceremony(parsed.Response.CollectedClientData, enrolling, origin)
	if err != nil {
		return "", err
	}
	if c.person != by.ID {
		return "", fmt.Errorf("%w: this Passkey request was someone else's", ErrRefused)
	}
	credential, err := rp.CreateCredential(user{&s.persons[i]}, c.data, parsed)
	if err != nil {
		return "", passkeyRefused(err)
	}
	k := Passkey{
		ID:             credential.ID,
		PublicKey:      credential.PublicKey,
		Counter:        credential.Authenticator.SignCount,
		BackupEligible: credential.Flags.BackupEligible,
		AAGUID:         credential.Authenticator.AAGUID,
		Created:        s.now(),
	}
	if err := s.changePasskeys(i, func(ks []Passkey) []Passkey { return append(ks, k) }); err != nil {
		return "", err
	}
	p := personParty(s.persons[i])
	s.record(Entry{Event: PasskeyAdded, Actor: p, Subject: p, Detail: provider(k)})
	return k.ID.String(), nil
}

// RemovePasskey removes Passkey id of Person person, as they or a fresh
// Admin Person do, each fresh for step-up (ADR 0025).
func (s *Store) RemovePasskey(by Identity, person, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.whose(by, person, true)
	if err != nil {
		return err
	}
	j := slices.IndexFunc(s.persons[i].Passkeys, func(k Passkey) bool { return k.ID.String() == id })
	if j < 0 {
		return fmt.Errorf("passkey %q: %w", id, home.ErrNotFound)
	}
	k := s.persons[i].Passkeys[j]
	if err := s.changePasskeys(i, func(ks []Passkey) []Passkey { return slices.Delete(ks, j, j+1) }); err != nil {
		return err
	}
	s.record(Entry{Event: PasskeyRemoved, Actor: party(by), Subject: personParty(s.persons[i]), Detail: provider(k)})
	return nil
}

// provider is the detail naming k's provider in the Audit log, if known.
func provider(k Passkey) map[string]any {
	if name := Provider(k.AAGUID); name != "" {
		return map[string]any{"provider": name}
	}
	return nil
}
