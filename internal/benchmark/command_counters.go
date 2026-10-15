package benchmark

func (result *SaturationResult) calculatePhysicalCommands() {
	if result.DatabaseBefore == nil || result.DatabaseAfter == nil {
		return
	}
	result.PhysicalCommandUnavailable = mergeCommandUnavailable(result.PhysicalCommandUnavailable, result.DatabaseBefore.CommandUnavailable, result.DatabaseAfter.CommandUnavailable)
	result.PhysicalCommands = make(map[string]uint64)
	for name, after := range result.DatabaseAfter.Commands {
		before, found := result.DatabaseBefore.Commands[name]
		if !found || after < before {
			result.PhysicalCommandUnavailable = mergeCommandUnavailable(result.PhysicalCommandUnavailable, "unavailable or reset "+name)
			continue
		}
		result.PhysicalCommands[name] = after - before
	}
}
