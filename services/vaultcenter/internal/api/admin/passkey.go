package admin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// challengeStore holds pending WebAuthn challenges with TTL.
var challengeStore sync.Map

type pendingChallenge struct {
	challenge []byte
	origin    string
	rpID      string
	expiresAt time.Time
}

func storeChallengeForSession(sessionKey string, ch *pendingChallenge) {
	challengeStore.Store(sessionKey, ch)
}

func loadAndDeleteChallenge(sessionKey string) (*pendingChallenge, bool) {
	val, ok := challengeStore.LoadAndDelete(sessionKey)
	if !ok {
		return nil, false
	}
	ch, _ := val.(*pendingChallenge)
	if ch == nil || time.Now().After(ch.expiresAt) {
		return nil, false
	}
	return ch, true
}

func rpIDFromRequest(r *http.Request) string {
	host := r.Host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	return host
}

func originFromRequest(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// --- Registration Begin ---

func (h *Handler) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate challenge")
		return
	}

	rpID := rpIDFromRequest(r)
	origin := originFromRequest(r)

	session, _ := h.currentAdminSession(r)
	sessionKey := "register:" + session.SessionID
	storeChallengeForSession(sessionKey, &pendingChallenge{
		challenge: challenge,
		origin:    origin,
		rpID:      rpID,
		expiresAt: time.Now().Add(60 * time.Second),
	})

	// Collect existing credential IDs for excludeCredentials
	existingPasskeys, _ := h.deps.DB().ListAdminPasskeys()
	excludeCredentials := make([]map[string]any, 0, len(existingPasskeys))
	for _, pk := range existingPasskeys {
		ec := map[string]any{
			"type": "public-key",
			"id":   base64.RawURLEncoding.EncodeToString([]byte(pk.CredentialID)),
		}
		if pk.Transports != "" {
			ec["transports"] = strings.Split(pk.Transports, ",")
		}
		excludeCredentials = append(excludeCredentials, ec)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"publicKey": map[string]any{
			"challenge": base64.RawURLEncoding.EncodeToString(challenge),
			"rp": map[string]any{
				"name": "VeilKey VaultCenter",
				"id":   rpID,
			},
			"user": map[string]any{
				"id":          base64.RawURLEncoding.EncodeToString([]byte("admin")),
				"name":        "admin",
				"displayName": "VeilKey Admin",
			},
			"pubKeyCredParams": []map[string]any{
				{"type": "public-key", "alg": -7},
			},
			"authenticatorSelection": map[string]any{
				"authenticatorAttachment": "platform",
				"residentKey":             "preferred",
				"userVerification":        "required",
			},
			"timeout":            60000,
			"attestation":        "none",
			"excludeCredentials": excludeCredentials,
		},
	})
}

// --- Registration Finish ---

func (h *Handler) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID       string `json:"id"`
		RawID    string `json:"rawId"`
		Type     string `json:"type"`
		Response struct {
			ClientDataJSON    string `json:"clientDataJSON"`
			AttestationObject string `json:"attestationObject"`
		} `json:"response"`
		Name string `json:"name"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	session, _ := h.currentAdminSession(r)
	sessionKey := "register:" + session.SessionID
	pending, ok := loadAndDeleteChallenge(sessionKey)
	if !ok {
		respondError(w, http.StatusBadRequest, "challenge expired or not found")
		return
	}

	// Decode and validate clientDataJSON
	clientDataJSONBytes, err := base64RawURLDecode(req.Response.ClientDataJSON)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid clientDataJSON encoding")
		return
	}
	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(clientDataJSONBytes, &clientData); err != nil {
		respondError(w, http.StatusBadRequest, "invalid clientDataJSON")
		return
	}
	if clientData.Type != "webauthn.create" {
		respondError(w, http.StatusBadRequest, "invalid clientData type")
		return
	}
	expectedChallenge := base64.RawURLEncoding.EncodeToString(pending.challenge)
	if clientData.Challenge != expectedChallenge {
		respondError(w, http.StatusBadRequest, "challenge mismatch")
		return
	}
	if clientData.Origin != pending.origin {
		respondError(w, http.StatusBadRequest, "origin mismatch")
		return
	}

	// Decode attestationObject (CBOR)
	attestationBytes, err := base64RawURLDecode(req.Response.AttestationObject)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attestationObject encoding")
		return
	}
	authData, err := parseAttestationObject(attestationBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attestation data")
		return
	}

	// Verify rpIdHash
	rpIDHash := sha256.Sum256([]byte(pending.rpID))
	if len(authData) < 37 {
		respondError(w, http.StatusBadRequest, "authData too short")
		return
	}
	for i := 0; i < 32; i++ {
		if authData[i] != rpIDHash[i] {
			respondError(w, http.StatusBadRequest, "rpIdHash mismatch")
			return
		}
	}

	flags := authData[32]
	if flags&0x01 == 0 { // User Present
		respondError(w, http.StatusBadRequest, "user not present")
		return
	}

	signCount := binary.BigEndian.Uint32(authData[33:37])

	// Parse attested credential data (flags & 0x40)
	if flags&0x40 == 0 {
		respondError(w, http.StatusBadRequest, "no attested credential data")
		return
	}
	if len(authData) < 55 {
		respondError(w, http.StatusBadRequest, "authData too short for attested credential")
		return
	}

	aaguid := fmt.Sprintf("%x", authData[37:53])
	credIDLen := binary.BigEndian.Uint16(authData[53:55])
	if len(authData) < 55+int(credIDLen) {
		respondError(w, http.StatusBadRequest, "authData too short for credential ID")
		return
	}
	credentialID := authData[55 : 55+credIDLen]
	coseKeyBytes := authData[55+credIDLen:]

	// Parse COSE key
	pubKey, err := parseCOSEES256Key(coseKeyBytes)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid COSE key format")
		return
	}

	// Serialize public key for storage (uncompressed point: 0x04 + X + Y)
	ecdhKey, err := pubKey.ECDH()
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid public key format")
		return
	}
	pubKeyBytes := ecdhKey.Bytes()

	credIDEncoded := base64.RawURLEncoding.EncodeToString(credentialID)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Passkey " + time.Now().Format("2006-01-02")
	}

	passkey := &db.AdminPasskey{
		CredentialID: credIDEncoded,
		Name:         name,
		PublicKey:    pubKeyBytes,
		AAGUID:       aaguid,
		SignCount:    signCount,
		Transports:   "",
	}
	if err := h.deps.DB().SaveAdminPasskey(passkey); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save passkey")
		return
	}

	h.deps.SaveAuditEvent("admin_auth", "default", "passkey_register", "admin_session", session.SessionID, "", "admin_auth", nil, map[string]any{
		"credential_id": credIDEncoded,
		"name":          name,
	})

	passkeys, _ := h.deps.DB().ListAdminPasskeys()
	respondJSON(w, http.StatusOK, map[string]any{
		"status":   "registered",
		"passkeys": passkeys,
	})
}

// --- Login Begin ---

func (h *Handler) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	passkeys, err := h.deps.DB().ListAdminPasskeys()
	if err != nil || len(passkeys) == 0 {
		respondError(w, http.StatusBadRequest, "no passkeys registered")
		return
	}

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate challenge")
		return
	}

	rpID := rpIDFromRequest(r)
	origin := originFromRequest(r)

	sessionKey := "login:" + crypto.GenerateUUID()
	storeChallengeForSession(sessionKey, &pendingChallenge{
		challenge: challenge,
		origin:    origin,
		rpID:      rpID,
		expiresAt: time.Now().Add(60 * time.Second),
	})

	allowCredentials := make([]map[string]any, 0, len(passkeys))
	for _, pk := range passkeys {
		ac := map[string]any{
			"type": "public-key",
			"id":   pk.CredentialID,
		}
		if pk.Transports != "" {
			ac["transports"] = strings.Split(pk.Transports, ",")
		}
		allowCredentials = append(allowCredentials, ac)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"session_key": sessionKey,
		"publicKey": map[string]any{
			"challenge":        base64.RawURLEncoding.EncodeToString(challenge),
			"rpId":             rpID,
			"allowCredentials": allowCredentials,
			"userVerification": "required",
			"timeout":          60000,
		},
	})
}

// --- Login Finish ---

func (h *Handler) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionKey string `json:"session_key"`
		ID         string `json:"id"`
		RawID      string `json:"rawId"`
		Type       string `json:"type"`
		Response   struct {
			ClientDataJSON    string `json:"clientDataJSON"`
			AuthenticatorData string `json:"authenticatorData"`
			Signature         string `json:"signature"`
		} `json:"response"`
	}
	if err := decodeRequestJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pending, ok := loadAndDeleteChallenge(req.SessionKey)
	if !ok {
		respondError(w, http.StatusBadRequest, "challenge expired or not found")
		return
	}

	// Decode clientDataJSON
	clientDataJSONBytes, err := base64RawURLDecode(req.Response.ClientDataJSON)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid clientDataJSON encoding")
		return
	}
	var clientData struct {
		Type      string `json:"type"`
		Challenge string `json:"challenge"`
		Origin    string `json:"origin"`
	}
	if err := json.Unmarshal(clientDataJSONBytes, &clientData); err != nil {
		respondError(w, http.StatusBadRequest, "invalid clientDataJSON")
		return
	}
	if clientData.Type != "webauthn.get" {
		respondError(w, http.StatusBadRequest, "invalid clientData type")
		return
	}
	expectedChallenge := base64.RawURLEncoding.EncodeToString(pending.challenge)
	if clientData.Challenge != expectedChallenge {
		respondError(w, http.StatusBadRequest, "challenge mismatch")
		return
	}
	if clientData.Origin != pending.origin {
		respondError(w, http.StatusBadRequest, "origin mismatch")
		return
	}

	// Find the passkey
	credID := req.ID
	passkey, err := h.deps.DB().GetAdminPasskeyByID(credID)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "unknown credential")
		return
	}

	// Decode authenticator data
	authDataBytes, err := base64RawURLDecode(req.Response.AuthenticatorData)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid authenticatorData encoding")
		return
	}

	// Verify rpIdHash
	rpIDHash := sha256.Sum256([]byte(pending.rpID))
	if len(authDataBytes) < 37 {
		respondError(w, http.StatusBadRequest, "authenticatorData too short")
		return
	}
	for i := 0; i < 32; i++ {
		if authDataBytes[i] != rpIDHash[i] {
			respondError(w, http.StatusBadRequest, "rpIdHash mismatch")
			return
		}
	}

	flags := authDataBytes[32]
	if flags&0x01 == 0 {
		respondError(w, http.StatusBadRequest, "user not present")
		return
	}
	if flags&0x04 == 0 {
		respondError(w, http.StatusBadRequest, "user not verified")
		return
	}

	newSignCount := binary.BigEndian.Uint32(authDataBytes[33:37])
	if passkey.SignCount > 0 && newSignCount <= passkey.SignCount {
		respondError(w, http.StatusUnauthorized, "possible cloned authenticator")
		return
	}

	// Reconstruct public key
	pubKey, err := unmarshalECPublicKey(passkey.PublicKey)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "invalid stored public key")
		return
	}

	// Verify signature: signature is over SHA256(authData + SHA256(clientDataJSON))
	clientDataHash := sha256.Sum256(clientDataJSONBytes)
	signedData := make([]byte, 0, len(authDataBytes)+len(clientDataHash))
	signedData = append(signedData, authDataBytes...)
	signedData = append(signedData, clientDataHash[:]...)

	sigBytes, err := base64RawURLDecode(req.Response.Signature)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid signature encoding")
		return
	}

	hash := sha256.Sum256(signedData)
	if !ecdsa.VerifyASN1(pubKey, hash[:], sigBytes) {
		respondError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	// Update sign count
	_ = h.deps.DB().UpdatePasskeySignCount(credID, newSignCount)

	// Create admin session (same as TOTP login)
	token, tokenHash := generateAdminSessionToken()
	now := time.Now().UTC()
	adminSession := &db.AdminSession{
		SessionID:     crypto.GenerateUUID(),
		TokenHash:     tokenHash,
		AuthMethod:    "passkey",
		RemoteAddr:    httputil.ActorIDForRequest(r),
		ExpiresAt:     now.Add(adminSessionTTL()),
		IdleExpiresAt: now.Add(adminSessionIdleTimeout()),
		LastSeenAt:    now,
		CreatedAt:     now,
	}
	if err := h.deps.DB().SaveAdminSession(adminSession); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create admin session")
		return
	}
	setAdminSessionCookie(w, token, adminSession.ExpiresAt)

	h.deps.SaveAuditEvent("admin_session", adminSession.SessionID, "session_login", "api", httputil.ActorIDForRequest(r), "", "admin_auth", nil, map[string]any{
		"auth_method":     "passkey",
		"credential_id":   credID,
		"expires_at":      adminSession.ExpiresAt.Format(time.RFC3339),
		"idle_expires_at": adminSession.IdleExpiresAt.Format(time.RFC3339),
	})

	respondJSON(w, http.StatusOK, sessionPayload(adminSession))
}

// --- List Passkeys ---

func (h *Handler) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	passkeys, err := h.deps.DB().ListAdminPasskeys()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list passkeys")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"passkeys": passkeys,
	})
}

// --- Delete Passkey ---

func (h *Handler) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	credID := r.PathValue("id")
	if credID == "" {
		respondError(w, http.StatusBadRequest, "credential id is required")
		return
	}
	if err := h.deps.DB().DeleteAdminPasskey(credID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to delete passkey")
		return
	}
	session, _ := h.currentAdminSession(r)
	h.deps.SaveAuditEvent("admin_auth", "default", "passkey_delete", "admin_session", session.SessionID, "", "admin_auth", nil, map[string]any{
		"credential_id": credID,
	})
	respondJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

// --- Helper: base64 raw URL decode ---

func base64RawURLDecode(s string) ([]byte, error) {
	// Handle both padded and unpadded base64url
	s = strings.TrimRight(s, "=")
	return base64.RawURLEncoding.DecodeString(s)
}

// --- Minimal CBOR parser for attestation "none" format ---

func parseAttestationObject(data []byte) ([]byte, error) {
	// The attestation object is CBOR-encoded map with keys: "fmt", "attStmt", "authData"
	// For "none" format: {"fmt": "none", "attStmt": {}, "authData": <bytes>}
	// We need a minimal CBOR decoder.
	reader := &cborReader{data: data, pos: 0}
	result, err := reader.readMap()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CBOR: %w", err)
	}
	authDataVal, ok := result["authData"]
	if !ok {
		return nil, fmt.Errorf("authData not found in attestation object")
	}
	authData, ok := authDataVal.([]byte)
	if !ok {
		return nil, fmt.Errorf("authData is not a byte string")
	}
	return authData, nil
}

type cborReader struct {
	data []byte
	pos  int
}

func (c *cborReader) readByte() (byte, error) {
	if c.pos >= len(c.data) {
		return 0, fmt.Errorf("unexpected end of CBOR data")
	}
	b := c.data[c.pos]
	c.pos++
	return b, nil
}

func (c *cborReader) readBytes(n int) ([]byte, error) {
	if c.pos+n > len(c.data) {
		return nil, fmt.Errorf("unexpected end of CBOR data")
	}
	b := c.data[c.pos : c.pos+n]
	c.pos += n
	return b, nil
}

func (c *cborReader) readUint(additionalInfo byte) (uint64, error) {
	if additionalInfo < 24 {
		return uint64(additionalInfo), nil
	}
	switch additionalInfo {
	case 24:
		b, err := c.readByte()
		return uint64(b), err
	case 25:
		bs, err := c.readBytes(2)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint16(bs)), nil
	case 26:
		bs, err := c.readBytes(4)
		if err != nil {
			return 0, err
		}
		return uint64(binary.BigEndian.Uint32(bs)), nil
	case 27:
		bs, err := c.readBytes(8)
		if err != nil {
			return 0, err
		}
		return binary.BigEndian.Uint64(bs), nil
	}
	return 0, fmt.Errorf("unsupported CBOR additional info: %d", additionalInfo)
}

func (c *cborReader) readValue() (any, error) {
	b, err := c.readByte()
	if err != nil {
		return nil, err
	}
	majorType := b >> 5
	additionalInfo := b & 0x1f

	switch majorType {
	case 0: // unsigned integer
		val, err := c.readUint(additionalInfo)
		return int64(val), err
	case 1: // negative integer
		val, err := c.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		return -1 - int64(val), err
	case 2: // byte string
		length, err := c.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		return c.readBytes(int(length))
	case 3: // text string
		length, err := c.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		bs, err := c.readBytes(int(length))
		if err != nil {
			return nil, err
		}
		return string(bs), nil
	case 4: // array
		length, err := c.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		arr := make([]any, length)
		for i := uint64(0); i < length; i++ {
			arr[i], err = c.readValue()
			if err != nil {
				return nil, err
			}
		}
		return arr, nil
	case 5: // map
		length, err := c.readUint(additionalInfo)
		if err != nil {
			return nil, err
		}
		m := make(map[any]any, length)
		for i := uint64(0); i < length; i++ {
			key, err := c.readValue()
			if err != nil {
				return nil, err
			}
			val, err := c.readValue()
			if err != nil {
				return nil, err
			}
			m[key] = val
		}
		return m, nil
	case 7: // simple/float
		if additionalInfo == 20 {
			return false, nil
		}
		if additionalInfo == 21 {
			return true, nil
		}
		if additionalInfo == 22 {
			return nil, nil
		}
		return nil, fmt.Errorf("unsupported CBOR simple value: %d", additionalInfo)
	}
	return nil, fmt.Errorf("unsupported CBOR major type: %d", majorType)
}

func (c *cborReader) readMap() (map[string]any, error) {
	val, err := c.readValue()
	if err != nil {
		return nil, err
	}
	// The CBOR map may have string or integer keys
	raw, ok := val.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("expected CBOR map")
	}
	result := make(map[string]any, len(raw))
	for k, v := range raw {
		switch kk := k.(type) {
		case string:
			result[kk] = v
		case int64:
			result[fmt.Sprintf("%d", kk)] = v
		default:
			result[fmt.Sprintf("%v", kk)] = v
		}
	}
	return result, nil
}

// --- COSE Key Parser (ES256 only) ---

func parseCOSEES256Key(data []byte) (*ecdsa.PublicKey, error) {
	reader := &cborReader{data: data, pos: 0}
	val, err := reader.readValue()
	if err != nil {
		return nil, fmt.Errorf("failed to parse COSE key CBOR: %w", err)
	}
	raw, ok := val.(map[any]any)
	if !ok {
		return nil, fmt.Errorf("COSE key is not a map")
	}

	// Convert to int64 keyed map
	m := make(map[int64]any, len(raw))
	for k, v := range raw {
		switch kk := k.(type) {
		case int64:
			m[kk] = v
		}
	}

	// Verify key type (1 = kty) == 2 (EC2)
	kty, _ := m[1].(int64)
	if kty != 2 {
		return nil, fmt.Errorf("unsupported key type: %d (expected EC2=2)", kty)
	}

	// Verify algorithm (3 = alg) == -7 (ES256)
	alg, _ := m[3].(int64)
	if alg != -7 {
		return nil, fmt.Errorf("unsupported algorithm: %d (expected ES256=-7)", alg)
	}

	// Verify curve (-1 = crv) == 1 (P-256)
	crv, _ := m[-1].(int64)
	if crv != 1 {
		return nil, fmt.Errorf("unsupported curve: %d (expected P-256=1)", crv)
	}

	// Extract X (-2) and Y (-3)
	xBytes, ok := m[-2].([]byte)
	if !ok || len(xBytes) != 32 {
		return nil, fmt.Errorf("invalid X coordinate")
	}
	yBytes, ok := m[-3].([]byte)
	if !ok || len(yBytes) != 32 {
		return nil, fmt.Errorf("invalid Y coordinate")
	}

	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}

	if !pubKey.IsOnCurve(pubKey.X, pubKey.Y) {
		return nil, fmt.Errorf("public key point is not on curve")
	}

	return pubKey, nil
}

// --- Unmarshal stored EC public key ---

func unmarshalECPublicKey(data []byte) (*ecdsa.PublicKey, error) {
	// Stored as uncompressed: 0x04 + X(32) + Y(32) = 65 bytes
	if len(data) != 65 || data[0] != 0x04 {
		return nil, fmt.Errorf("invalid uncompressed EC public key")
	}
	x := new(big.Int).SetBytes(data[1:33])
	y := new(big.Int).SetBytes(data[33:65])
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}
	if !pubKey.IsOnCurve(pubKey.X, pubKey.Y) {
		return nil, fmt.Errorf("public key point is not on curve")
	}
	return pubKey, nil
}
