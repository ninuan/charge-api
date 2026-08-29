package model

// NotificationActionLifecycle separates one-shot information from problems
// that remain actionable until the underlying condition recovers.
type NotificationActionLifecycle string

const (
	NotificationInformational  NotificationActionLifecycle = "informational"
	NotificationActionRequired NotificationActionLifecycle = "action_required"
)

// ActionLifecycle is the domain definition that later notification filtering
// and presentation changes must share. Read state is intentionally independent
// from whether a notification represents an unresolved problem.
func (notificationType NotificationType) ActionLifecycle() NotificationActionLifecycle {
	switch notificationType {
	case NotificationCredentialExpired, NotificationPileOffline:
		return NotificationActionRequired
	case NotificationPileAvailable, NotificationPileRecovered:
		return NotificationInformational
	default:
		return NotificationInformational
	}
}

func (notificationType NotificationType) RequiresAction() bool {
	return notificationType.ActionLifecycle() == NotificationActionRequired
}

// NotificationDeliveryUserState preserves facts the system has already
// established. In particular, a missing provider confirmation must not erase
// a successful submission recorded by AcceptedAt.
type NotificationDeliveryUserState string

const (
	NotificationDeliveryUserQueued     NotificationDeliveryUserState = "queued"
	NotificationDeliveryUserSubmitted  NotificationDeliveryUserState = "submitted"
	NotificationDeliveryUserProcessed  NotificationDeliveryUserState = "processed"
	NotificationDeliveryUserSuppressed NotificationDeliveryUserState = "suppressed"
	NotificationDeliveryUserFailed     NotificationDeliveryUserState = "failed"
	NotificationDeliveryUserCancelled  NotificationDeliveryUserState = "cancelled"
	NotificationDeliveryUserUnknown    NotificationDeliveryUserState = "unknown"
)

func (delivery NotificationDelivery) UserState() NotificationDeliveryUserState {
	switch delivery.Status {
	case NotificationDeliveryPending, NotificationDeliverySending, NotificationDeliveryRetryWait:
		return NotificationDeliveryUserQueued
	case NotificationDeliveryAccepted:
		return NotificationDeliveryUserSubmitted
	case NotificationDeliveryProviderSucceeded:
		return NotificationDeliveryUserProcessed
	case NotificationDeliverySuppressed:
		return NotificationDeliveryUserSuppressed
	case NotificationDeliveryFailed:
		return NotificationDeliveryUserFailed
	case NotificationDeliveryCancelled:
		return NotificationDeliveryUserCancelled
	case NotificationDeliveryUncertain:
		if delivery.AcceptedAt != nil {
			return NotificationDeliveryUserSubmitted
		}
		return NotificationDeliveryUserUnknown
	default:
		return NotificationDeliveryUserUnknown
	}
}
