package api

func IsTerminalPhase(phase AssignmentPhase) bool {
	switch phase {
	case AssignmentPhaseSucceeded, AssignmentPhaseFailed, AssignmentPhaseStopped, AssignmentPhaseLost:
		return true
	default:
		return false
	}
}

func IsActivePhase(phase AssignmentPhase) bool {
	return phase == AssignmentPhasePending || phase == AssignmentPhaseRunning
}
