package me

type OnboardingStatus string

const (
	OnboardingStatusNotStarted OnboardingStatus = "not_started"
	OnboardingStatusInProgress OnboardingStatus = "in_progress"
	OnboardingStatusCompleted  OnboardingStatus = "completed"
	OnboardingStatusDismissed  OnboardingStatus = "dismissed"
)

func (s OnboardingStatus) String() string {
	return string(s)
}

type OnboardingAction string

const (
	OnboardingActionAdvance  OnboardingAction = "advance"
	OnboardingActionRetreat  OnboardingAction = "retreat"
	OnboardingActionSkip     OnboardingAction = "skip"
	OnboardingActionDismiss  OnboardingAction = "dismiss"
	OnboardingActionComplete OnboardingAction = "complete"
)

func (a OnboardingAction) String() string {
	return string(a)
}

type ConsentPurpose string

const (
	ConsentPurposePodologicalProfile ConsentPurpose = "podological_profile"
	ConsentPurposePrescription       ConsentPurpose = "prescription"
	ConsentPurposeCustomOrder        ConsentPurpose = "custom_order"
)

func (p ConsentPurpose) String() string {
	return string(p)
}

var currentTermsVersions = map[ConsentPurpose]string{
	ConsentPurposePodologicalProfile: "podological-profile-v1",
	ConsentPurposePrescription:       "prescription-v1",
	ConsentPurposeCustomOrder:        "custom-order-v1",
}

type FootstrikeType string

const (
	FootstrikeTypeNeutral   FootstrikeType = "neutral"
	FootstrikeTypePronated  FootstrikeType = "pronated"
	FootstrikeTypeSupinated FootstrikeType = "supinated"
	FootstrikeTypeUnknown   FootstrikeType = "unknown"
)

func (f FootstrikeType) IsValid() bool {
	switch f {
	case FootstrikeTypeNeutral, FootstrikeTypePronated, FootstrikeTypeSupinated, FootstrikeTypeUnknown:
		return true
	}
	return false
}
