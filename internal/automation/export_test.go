package automation

// SpaceRuns gives e a clock whose readings are a second apart, which keeps
// the runaway guard out of the way of many Runs.
func SpaceRuns(e *Engine) { e.now = spaced() }
