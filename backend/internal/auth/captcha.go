package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"html"
	"strings"
	"sync"
	"time"
)

const (
	captchaTTL       = 5 * time.Minute
	captchaMaxAnswer = 12
)

var captchaAlphabet = []byte("23456789ABCDEFGHJKLMNPQRSTUVWXYZ")

type CaptchaChallenge struct {
	ID        string    `json:"id"`
	Image     string    `json:"image"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type captchaEntry struct {
	answer    string
	expiresAt time.Time
}

type CaptchaStore struct {
	mu         sync.Mutex
	challenges map[string]captchaEntry
	now        func() time.Time
}

func NewCaptchaStore() *CaptchaStore {
	return &CaptchaStore{
		challenges: make(map[string]captchaEntry),
		now:        time.Now,
	}
}

func (s *CaptchaStore) Generate() (CaptchaChallenge, error) {
	if s == nil {
		return CaptchaChallenge{}, fmt.Errorf("captcha store is not configured")
	}

	idBytes := make([]byte, 18)
	if _, err := rand.Read(idBytes); err != nil {
		return CaptchaChallenge{}, fmt.Errorf("generate captcha id: %w", err)
	}
	answerBytes := make([]byte, 5)
	randomBytes := make([]byte, len(answerBytes))
	if _, err := rand.Read(randomBytes); err != nil {
		return CaptchaChallenge{}, fmt.Errorf("generate captcha answer: %w", err)
	}
	for i, value := range randomBytes {
		answerBytes[i] = captchaAlphabet[int(value)%len(captchaAlphabet)]
	}

	id := base64.RawURLEncoding.EncodeToString(idBytes)
	answer := string(answerBytes)
	expiresAt := s.now().Add(captchaTTL)
	image := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(captchaSVG(answer, randomBytes)))

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(s.now())
	s.challenges[id] = captchaEntry{answer: answer, expiresAt: expiresAt}

	return CaptchaChallenge{ID: id, Image: image, ExpiresAt: expiresAt}, nil
}

func (s *CaptchaStore) Verify(id string, answer string) error {
	if s == nil {
		return fmt.Errorf("验证码服务不可用")
	}
	id = strings.TrimSpace(id)
	answer = strings.ToUpper(strings.TrimSpace(answer))
	if id == "" || answer == "" {
		return fmt.Errorf("请输入验证码")
	}
	if len(answer) > captchaMaxAnswer {
		return fmt.Errorf("验证码无效")
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.challenges[id]
	delete(s.challenges, id)
	s.cleanupLocked(now)
	if !ok || now.After(entry.expiresAt) {
		return fmt.Errorf("验证码已过期，请刷新后重试")
	}
	if answer != entry.answer {
		return fmt.Errorf("验证码错误，请重试")
	}
	return nil
}

func (s *CaptchaStore) cleanupLocked(now time.Time) {
	for id, entry := range s.challenges {
		if now.After(entry.expiresAt) {
			delete(s.challenges, id)
		}
	}
}

func captchaSVG(answer string, seed []byte) string {
	escaped := html.EscapeString(answer)
	lineA := int(seed[0]%42) + 8
	lineB := int(seed[1]%34) + 20
	lineC := int(seed[2]%48) + 90
	lineD := int(seed[3]%34) + 26
	dotA := int(seed[4]%110) + 10

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="168" height="54" viewBox="0 0 168 54" role="img" aria-label="captcha">
<defs>
  <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0" stop-color="#ecfdf5"/>
    <stop offset="1" stop-color="#fef3c7"/>
  </linearGradient>
</defs>
<rect width="168" height="54" rx="14" fill="url(#bg)"/>
<path d="M8 %d C42 %d, 86 %d, 160 %d" fill="none" stroke="#2f6f4f" stroke-width="2.6" stroke-linecap="round" opacity=".42"/>
<path d="M6 41 C50 18, 110 50, 164 22" fill="none" stroke="#b45309" stroke-width="1.8" stroke-linecap="round" opacity=".28"/>
<circle cx="%d" cy="14" r="3" fill="#2f6f4f" opacity=".28"/>
<circle cx="126" cy="39" r="2.5" fill="#b45309" opacity=".25"/>
<text x="84" y="36" text-anchor="middle" font-family="ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace" font-size="25" font-weight="800" letter-spacing="5" fill="#14231b" transform="rotate(-3 84 27)">%s</text>
</svg>`, lineA, lineB, lineC, lineD, dotA, escaped)
}
