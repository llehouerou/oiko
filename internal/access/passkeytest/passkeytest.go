// Package passkeytest is a software authenticator for tests: it holds one
// Passkey and answers WebAuthn ceremonies as a browser and its passkey
// provider would, with attestation "none".
package passkeytest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// AAGUID is the provider this authenticator says it is: Google Password
// Manager's.
var AAGUID = [16]byte{0xea, 0x9b, 0x8d, 0x66, 0x4d, 0x01, 0x1d, 0x21, 0x3c, 0xe4, 0xb6, 0xb4, 0x8c, 0xb5, 0x75, 0xd4}

// Authenticator is a browser at Origin with a passkey provider holding one
// Passkey, made by Create.
type Authenticator struct {
	Origin             string // what the browser signs as the ceremony's origin
	NoUserVerification bool   // its answers do not say the user was verified
	Counter            uint32 // the signature counter its next answer reports

	key        *ecdsa.PrivateKey
	id         []byte
	rpID, user []byte
}

// New is an authenticator without a Passkey yet, in a browser at origin.
func New(origin string) *Authenticator {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return &Authenticator{Origin: origin, key: key, id: []byte(rand.Text())}
}

// Create answers options, the creation options a Relying Party sent (as JSON,
// or a value encoding to it), with a new Passkey, which it keeps.
func (a *Authenticator) Create(t testing.TB, options any) []byte {
	t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
			RP        struct {
				ID string `json:"id"`
			} `json:"rp"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
		} `json:"publicKey"`
	}
	decode(t, options, &o)
	a.rpID = []byte(o.PublicKey.RP.ID)
	a.user = unbase64(t, o.PublicKey.User.ID)
	key, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{KeyType: int64(webauthncose.EllipticKey), Algorithm: int64(webauthncose.AlgES256)},
		Curve:         int64(webauthncose.P256),
		XCoord:        a.key.X.FillBytes(make([]byte, 32)),
		YCoord:        a.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	data := a.authData(0x40) // attested credential data included
	data = append(data, AAGUID[:]...)
	data = binary.BigEndian.AppendUint16(data, uint16(len(a.id)))
	data = append(append(data, a.id...), key...)
	object, err := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": data})
	if err != nil {
		t.Fatal(err)
	}
	return a.answer(t, map[string]any{
		"clientDataJSON":    a.clientData(t, "webauthn.create", o.PublicKey.Challenge),
		"attestationObject": object,
		"transports":        []string{"internal", "hybrid"},
	})
}

// Get answers options, the request options a Relying Party sent, with its
// Passkey's assertion.
func (a *Authenticator) Get(t testing.TB, options any) []byte {
	t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	decode(t, options, &o)
	data := a.authData(0)
	client := a.clientData(t, "webauthn.get", o.PublicKey.Challenge)
	sum := sha256.Sum256(client)
	digest := sha256.Sum256(append(data, sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, a.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return a.answer(t, map[string]any{
		"clientDataJSON":    client,
		"authenticatorData": data,
		"signature":         sig,
		"userHandle":        a.user,
	})
}

// authData is the authenticator data up to its counter, with flags beyond
// user present and verified.
func (a *Authenticator) authData(flags byte) []byte {
	rp := sha256.Sum256(a.rpID)
	flags |= 0x01 // user present
	if !a.NoUserVerification {
		flags |= 0x04
	}
	return binary.BigEndian.AppendUint32(append(rp[:], flags), a.Counter)
}

func (a *Authenticator) clientData(t testing.TB, kind, challenge string) []byte {
	data, err := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": a.Origin, "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// answer is the PublicKeyCredential a browser sends as JSON, its byte
// strings in base64url.
func (a *Authenticator) answer(t testing.TB, response map[string]any) []byte {
	for k, v := range response {
		if b, ok := v.([]byte); ok {
			response[k] = base64.RawURLEncoding.EncodeToString(b)
		}
	}
	id := base64.RawURLEncoding.EncodeToString(a.id)
	data, err := json.Marshal(map[string]any{
		"id": id, "rawId": id, "type": "public-key", "authenticatorAttachment": "platform",
		"response": response, "clientExtensionResults": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decode(t testing.TB, options any, v any) {
	data, ok := options.([]byte)
	if !ok {
		var err error
		if data, err = json.Marshal(options); err != nil {
			t.Fatal(err)
		}
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func unbase64(t testing.TB, s string) []byte {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
