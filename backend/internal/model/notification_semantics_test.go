package model

import (
	"testing"
	"time"
)

func TestNotificationActionLifecycleContract(t *testing.T) {
	tests := []struct {
		name           string
		notification   NotificationType
		wantLifecycle  NotificationActionLifecycle
		wantActionable bool
	}{
		{name: "available is one-shot information", notification: NotificationPileAvailable, wantLifecycle: NotificationInformational},
		{name: "recovery is one-shot information", notification: NotificationPileRecovered, wantLifecycle: NotificationInformational},
		{name: "expired credential needs action", notification: NotificationCredentialExpired, wantLifecycle: NotificationActionRequired, wantActionable: true},
		{name: "offline pile needs action", notification: NotificationPileOffline, wantLifecycle: NotificationActionRequired, wantActionable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.notification.ActionLifecycle(); got != tt.wantLifecycle {
				t.Fatalf("ActionLifecycle() = %q, want %q", got, tt.wantLifecycle)
			}
			if got := tt.notification.RequiresAction(); got != tt.wantActionable {
				t.Fatalf("RequiresAction() = %v, want %v", got, tt.wantActionable)
			}
		})
	}
}

func TestNotificationDeliveryUserStatePreservesAcceptedSubmission(t *testing.T) {
	acceptedAt := time.Date(2026, 8, 26, 13, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		delivery NotificationDelivery
		want     NotificationDeliveryUserState
	}{
		{
			name: "provider query unknown after accepted submission",
			delivery: NotificationDelivery{
				Status: NotificationDeliveryUncertain, AcceptedAt: &acceptedAt,
				LastErrorCode: "provider_status_unknown",
			},
			want: NotificationDeliveryUserSubmitted,
		},
		{
			name: "send outcome unknown without acceptance",
			delivery: NotificationDelivery{
				Status:        NotificationDeliveryUncertain,
				LastErrorCode: "send_outcome_unknown",
			},
			want: NotificationDeliveryUserUnknown,
		},
		{name: "provider processed", delivery: NotificationDelivery{Status: NotificationDeliveryProviderSucceeded}, want: NotificationDeliveryUserProcessed},
		{name: "explicit failure", delivery: NotificationDelivery{Status: NotificationDeliveryFailed}, want: NotificationDeliveryUserFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.delivery.UserState(); got != tt.want {
				t.Fatalf("UserState() = %q, want %q", got, tt.want)
			}
		})
	}
}
