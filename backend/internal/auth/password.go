package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Version = "argon2id"
	argon2Memory  = 64 * 1024
	argon2Time    = 3
	argon2Threads = 4
	argon2KeyLen  = 32
	saltLength    = 16
)

func HashPassword(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("password is required")
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)
	return fmt.Sprintf(
		"%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		argon2.Version,
		argon2Memory,
		argon2Time,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password string, encoded string) (valid bool, needsUpgrade bool) {
	if strings.HasPrefix(encoded, argon2Version+"$") {
		return verifyArgon2(password, encoded), false
	}
	if strings.HasPrefix(encoded, wrappedLegacyPrefix) {
		return verifyWrappedLegacy(password, encoded), true
	}
	if IsLegacySHA256(encoded) {
		return verifyLegacySHA256(password, encoded), true
	}
	return false, false
}

const wrappedLegacyPrefix = "wrapped-sha256$"

func IsLegacySHA256(encoded string) bool {
	return strings.HasPrefix(encoded, "sha256$")
}

// WrapLegacySHA256 在不知道明文密码的情况下，把 sha256$ 旧哈希的摘要整体
// 套上 argon2id。数据库泄露时不再存在可离线快速爆破的弱哈希；用户下次
// 登录成功后仍会照常升级为纯 argon2id。
func WrapLegacySHA256(encoded string) (string, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		return "", fmt.Errorf("not a legacy sha256 hash")
	}
	inner, err := HashPassword(parts[2])
	if err != nil {
		return "", err
	}
	return wrappedLegacyPrefix + parts[1] + "$" + inner, nil
}

func verifyWrappedLegacy(password string, encoded string) bool {
	rest := strings.TrimPrefix(encoded, wrappedLegacyPrefix)
	saltEncoded, inner, found := strings.Cut(rest, "$")
	if !found {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(saltEncoded)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(append(salt, []byte(password)...))
	return verifyArgon2(base64.RawStdEncoding.EncodeToString(sum[:]), inner)
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// VerifyDummyPassword 供"用户不存在"分支调用：跑一次与真实校验同参数的
// argon2id 计算，消除可用于枚举用户名的响应时间差。
func VerifyDummyPassword(password string) {
	dummyHashOnce.Do(func() {
		if hash, err := HashPassword("charge-timing-equalizer"); err == nil {
			dummyHash = hash
		}
	})
	if dummyHash != "" {
		verifyArgon2(password, dummyHash)
	}
}

func verifyArgon2(password string, encoded string) bool {
	var version int
	var memory uint32
	var iterations uint32
	var threads uint8

	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != argon2Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[1], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) == 0 {
		return false
	}

	actual := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func verifyLegacySHA256(password string, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	sum := sha256.Sum256(append(salt, []byte(password)...))
	return subtle.ConstantTimeCompare(sum[:], expected) == 1
}
