package wxpusher

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	ErrorUnconfigured        ErrorCode = "wxpusher_unconfigured"
	ErrorInvalidRequest      ErrorCode = "wxpusher_invalid_request"
	ErrorInvalidToken        ErrorCode = "wxpusher_invalid_token"
	ErrorInvalidUID          ErrorCode = "wxpusher_invalid_uid"
	ErrorRecipientRejected   ErrorCode = "wxpusher_recipient_rejected"
	ErrorRateLimited         ErrorCode = "wxpusher_rate_limited"
	ErrorProviderTimeout     ErrorCode = "wxpusher_provider_timeout"
	ErrorProviderUnavailable ErrorCode = "wxpusher_provider_unavailable"
	ErrorInvalidResponse     ErrorCode = "wxpusher_invalid_response"
	ErrorProviderRejected    ErrorCode = "wxpusher_provider_rejected"
)

type Error struct {
	Code         ErrorCode
	Operation    string
	Retryable    bool
	HTTPStatus   int
	ProviderCode int
}

// Error deliberately excludes provider messages, request URLs and identifiers.
// Callers may safely write it to structured logs without exposing tokens or UIDs.
func (e *Error) Error() string {
	if e == nil {
		return "wxpusher request failed"
	}
	return fmt.Sprintf("wxpusher %s failed: code=%s", e.Operation, e.Code)
}

func CodeOf(err error) ErrorCode {
	var providerErr *Error
	if errors.As(err, &providerErr) {
		return providerErr.Code
	}
	return ""
}

func IsRetryable(err error) bool {
	var providerErr *Error
	return errors.As(err, &providerErr) && providerErr.Retryable
}

func newClientError(code ErrorCode, operation string, retryable bool, httpStatus, providerCode int, _ error) *Error {
	// The underlying error can contain a request URL (including a QR code) or
	// provider response fragments. Classify it before this call, then discard it.
	return &Error{
		Code:         code,
		Operation:    operation,
		Retryable:    retryable,
		HTTPStatus:   httpStatus,
		ProviderCode: providerCode,
	}
}

func classifyBusinessError(operation string, providerCode int, providerMessage string) *Error {
	message := strings.ToLower(strings.TrimSpace(providerMessage))
	switch {
	case containsAny(message, "apptoken", "app token", "token无效", "token错误", "token不存在", "token不正确", "token为空", "应用不存在", "应用令牌"):
		return newClientError(ErrorInvalidToken, operation, false, 0, providerCode, nil)
	case containsAny(message, "拒收", "拒绝接收", "已拉黑", "被拉黑", "未关注", "取消关注", "recipient rejected"):
		return newClientError(ErrorRecipientRejected, operation, false, 0, providerCode, nil)
	case (strings.Contains(message, "uid") && containsAny(message, "无效", "错误", "不存在", "invalid", "not found")) || containsAny(message, "用户不存在", "用户无效"):
		return newClientError(ErrorInvalidUID, operation, false, 0, providerCode, nil)
	case containsAny(message, "限流", "频率", "请求过多", "too many", "rate limit"):
		return newClientError(ErrorRateLimited, operation, true, 0, providerCode, nil)
	case containsAny(message, "timeout", "超时"):
		return newClientError(ErrorProviderTimeout, operation, true, 0, providerCode, nil)
	case containsAny(message, "系统繁忙", "服务不可用", "系统异常", "server error", "service unavailable"):
		return newClientError(ErrorProviderUnavailable, operation, true, 0, providerCode, nil)
	default:
		return newClientError(ErrorProviderRejected, operation, false, 0, providerCode, nil)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
