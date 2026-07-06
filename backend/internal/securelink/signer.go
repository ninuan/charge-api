package securelink

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	HeaderTimestamp = "X-YYB-Timestamp"
	HeaderNonce     = "X-YYB-Nonce"
	HeaderSignature = "X-YYB-Signature"
)

type Signer struct {
	secret []byte
	now    func() time.Time
}

func NewSigner(secret []byte) (*Signer, error) {
	if len(secret) == 0 {
		return nil, errors.New("YYB_API_SECRET is required")
	}
	copied := append([]byte(nil), secret...)
	return &Signer{secret: copied, now: time.Now}, nil
}

func (s *Signer) Sign(req *http.Request, body []byte) error {
	if s == nil || len(s.secret) == 0 {
		return errors.New("YYB signer is not configured")
	}
	nonce, err := randomNonce()
	if err != nil {
		return err
	}
	timestamp := fmt.Sprintf("%d", s.now().Unix())
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, HMACSignature(s.secret, req.Method, req.URL.RequestURI(), timestamp, nonce, body))
	return nil
}

func LoopbackBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" {
		return nil, fmt.Errorf("yyb base URL must use http loopback")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("yyb base URL missing host")
	}
	host := parsed.Hostname()
	if parsed.Port() == "" {
		return nil, fmt.Errorf("yyb base URL must include an explicit port")
	}
	if strings.EqualFold(host, "localhost") {
		return parsed, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("yyb base URL must be loopback")
	}
	return parsed, nil
}

func HMACSignature(secret []byte, method, pathQuery, timestamp, nonce string, body []byte) string {
	bodyHash := sha256.Sum256(body)
	canonical := strings.ToUpper(method) + "\n" + pathQuery + "\n" + timestamp + "\n" + nonce + "\n" + hex.EncodeToString(bodyHash[:])
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomNonce() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
