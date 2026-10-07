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
