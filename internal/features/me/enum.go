package me

type OnboardingStatus string

const (
	OnboardingStatusNotStarted OnboardingStatus = "nao_iniciado"
	OnboardingStatusInProgress OnboardingStatus = "em_andamento"
	OnboardingStatusCompleted  OnboardingStatus = "concluido"
	OnboardingStatusDismissed  OnboardingStatus = "dispensado"
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
