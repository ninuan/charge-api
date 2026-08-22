package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"time"
)

const (
	captchaTTL       = 2 * time.Minute
	captchaMaxAnswer = 12
	captchaWidth     = 168
	captchaHeight    = 54
)

// 易混淆的 0、1 不参与验证码；五位数字配合限流和失败锁定，足以阻止
// 自动化撞库，同时比混合大小写字母更适合手机输入。
var captchaAlphabet = []byte("23456789")

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
	randomBytes := make([]byte, 96)
	if _, err := rand.Read(randomBytes); err != nil {
		return CaptchaChallenge{}, fmt.Errorf("generate captcha answer: %w", err)
	}
	answerBytes := make([]byte, 5)
	for i := range answerBytes {
		answerBytes[i] = captchaAlphabet[int(randomBytes[i])%len(captchaAlphabet)]
	}

	id := base64.RawURLEncoding.EncodeToString(idBytes)
	answer := string(answerBytes)
	expiresAt := s.now().Add(captchaTTL)
	imageBytes, err := captchaPNG(answer, randomBytes[len(answerBytes):])
	if err != nil {
		return CaptchaChallenge{}, err
	}
	imageData := "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageBytes)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked(s.now())
	s.challenges[id] = captchaEntry{answer: answer, expiresAt: expiresAt}

	return CaptchaChallenge{ID: id, Image: imageData, ExpiresAt: expiresAt}, nil
}

func (s *CaptchaStore) Verify(id string, answer string) error {
	if s == nil {
		return fmt.Errorf("验证码服务不可用")
	}
	id = strings.TrimSpace(id)
	answer = strings.TrimSpace(answer)
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
	// 无论答案是否正确都立即销毁，避免同一挑战被重复试探。
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

var digitSegments = map[byte][]int{
	'2': {0, 1, 3, 4, 6},
	'3': {0, 1, 2, 3, 6},
	'4': {1, 2, 5, 6},
	'5': {0, 2, 3, 5, 6},
	'6': {0, 2, 3, 4, 5, 6},
	'7': {0, 1, 2},
	'8': {0, 1, 2, 3, 4, 5, 6},
	'9': {0, 1, 2, 3, 5, 6},
}

func captchaPNG(answer string, seed []byte) ([]byte, error) {
	canvas := image.NewNRGBA(image.Rect(0, 0, captchaWidth, captchaHeight))
	for y := 0; y < captchaHeight; y++ {
		for x := 0; x < captchaWidth; x++ {
			canvas.SetNRGBA(x, y, color.NRGBA{
				R: uint8(239 + (x+y)%9),
				G: uint8(246 + (x*2+y)%8),
				B: uint8(239 + (x+y*2)%10),
				A: 255,
			})
		}
	}

	noiseColors := []color.NRGBA{
		{R: 47, G: 111, B: 79, A: 105},
		{R: 180, G: 83, B: 9, A: 85},
		{R: 70, G: 92, B: 80, A: 72},
	}
	for i := 0; i < 3; i++ {
		offset := i * 4
		y1 := 7 + int(seed[offset]%40)
		y2 := 7 + int(seed[offset+1]%40)
		drawLine(canvas, 3, y1, captchaWidth-4, y2, noiseColors[i], 1)
	}
	for i := 12; i+2 < len(seed); i += 3 {
		x := int(seed[i]) % captchaWidth
		y := int(seed[i+1]) % captchaHeight
		canvas.SetNRGBA(x, y, noiseColors[int(seed[i+2])%len(noiseColors)])
	}

	glyphColors := []color.NRGBA{
		{R: 20, G: 35, B: 27, A: 255},
		{R: 30, G: 83, B: 58, A: 255},
		{R: 92, G: 61, B: 24, A: 255},
	}
	for index := 0; index < len(answer); index++ {
		seedIndex := 24 + index*3
		x := 9 + index*31 + int(seed[seedIndex]%5) - 2
		y := 6 + int(seed[seedIndex+1]%5) - 2
		paintDigit(canvas, answer[index], x, y, glyphColors[int(seed[seedIndex+2])%len(glyphColors)])
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return nil, fmt.Errorf("encode captcha image: %w", err)
	}
	return encoded.Bytes(), nil
}

func paintDigit(canvas *image.NRGBA, digit byte, x, y int, ink color.NRGBA) {
	segments := [7][4]int{
		{x + 4, y, x + 20, y},
		{x + 22, y + 3, x + 22, y + 18},
		{x + 22, y + 23, x + 22, y + 38},
		{x + 4, y + 41, x + 20, y + 41},
		{x + 2, y + 23, x + 2, y + 38},
		{x + 2, y + 3, x + 2, y + 18},
		{x + 4, y + 20, x + 20, y + 20},
	}
	for _, segment := range digitSegments[digit] {
		line := segments[segment]
		drawLine(canvas, line[0], line[1], line[2], line[3], ink, 3)
	}
}

func drawLine(canvas *image.NRGBA, x0, y0, x1, y1 int, ink color.NRGBA, thickness int) {
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		for oy := -thickness / 2; oy <= thickness/2; oy++ {
			for ox := -thickness / 2; ox <= thickness/2; ox++ {
				if image.Pt(x0+ox, y0+oy).In(canvas.Bounds()) {
					blendNRGBA(canvas, x0+ox, y0+oy, ink)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func blendNRGBA(canvas *image.NRGBA, x, y int, source color.NRGBA) {
	if source.A == 255 {
		canvas.SetNRGBA(x, y, source)
		return
	}
	destination := canvas.NRGBAAt(x, y)
	alpha := uint16(source.A)
	inverse := uint16(255 - source.A)
	canvas.SetNRGBA(x, y, color.NRGBA{
		R: uint8((uint16(source.R)*alpha + uint16(destination.R)*inverse) / 255),
		G: uint8((uint16(source.G)*alpha + uint16(destination.G)*inverse) / 255),
		B: uint8((uint16(source.B)*alpha + uint16(destination.B)*inverse) / 255),
		A: 255,
	})
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
