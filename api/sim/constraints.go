package sim

// ApplyConstraints updates attributes by one iteration.
func ApplyConstraints(update *Update, organism *Organism) {
	updateEnergy(update, organism)
	updateState(update, organism)
}

func updateEnergy(update *Update, organism *Organism) {
	state := update.State
	sizeFactor := state.Size * 0.1
	update.State.Energy = state.Energy - (0.01 + sizeFactor)
}

func updateState(update *Update, organism *Organism) {
	state := update.State
	if state.Energy <= 0 {
		// update state is dead
		update.State.Type = "dead"
	}
}
