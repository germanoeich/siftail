package core

// VisiblePlan defines the criteria for determining which log events should be visible
type VisiblePlan struct {
	Include       *Filters               // Include/exclude filters from Filters
	LevelMap      *LevelMap              // Severity level mapping and enabled state
	DockerVisible map[string]bool        // Container visibility by name or id (empty means all visible)
}

// ComputeVisible returns a filtered slice of events that should be visible
// based on the visibility plan. The returned slice contains references to
// the original events (no copying of event data).
func ComputeVisible(events []LogEvent, plan VisiblePlan) []LogEvent {
	if len(events) == 0 {
		return nil
	}

	result := make([]LogEvent, 0, len(events))
	
	for _, event := range events {
		if ShouldShowEvent(event, plan) {
			result = append(result, event)
		}
	}
	
	return result
}

// ShouldShowEvent determines if a single event should be visible based on the plan
func ShouldShowEvent(event LogEvent, plan VisiblePlan) bool {
	// 1. Check severity level enabled
	if plan.LevelMap != nil && !plan.LevelMap.IsEnabled(event.Level) {
		return false
	}
	
	// 2. Check Docker container visibility (only in docker mode)
	if plan.DockerVisible != nil && len(plan.DockerVisible) > 0 {
		if event.Source == SourceDocker {
			// Check visibility by container name first, then by ID
			visible, hasName := plan.DockerVisible[event.Container]
			if hasName && !visible {
				return false
			}
			// If not found by name and not visible by default, hide
			if !hasName && len(plan.DockerVisible) > 0 {
				// If DockerVisible is non-empty but doesn't contain this container,
				// default to not visible (explicit allow-list behavior)
				return false
			}
		}
	}
	
	// 3. Check include/exclude filters
	if plan.Include != nil && !plan.Include.ShouldShowLine(event.Line) {
		return false
	}
	
	return true
}

// FilterEventsByLevel returns events matching the enabled severity levels
func FilterEventsByLevel(events []LogEvent, levelMap *LevelMap) []LogEvent {
	if levelMap == nil {
		return events
	}
	
	result := make([]LogEvent, 0, len(events))
	for _, event := range events {
		if levelMap.IsEnabled(event.Level) {
			result = append(result, event)
		}
	}
	
	return result
}

// FilterEventsByContainer returns events from visible containers
func FilterEventsByContainer(events []LogEvent, dockerVisible map[string]bool) []LogEvent {
	if dockerVisible == nil || len(dockerVisible) == 0 {
		return events
	}
	
	result := make([]LogEvent, 0, len(events))
	for _, event := range events {
		if event.Source != SourceDocker {
			// Non-docker events are always visible
			result = append(result, event)
			continue
		}
		
		// Check visibility by container name
		if visible, exists := dockerVisible[event.Container]; exists && visible {
			result = append(result, event)
		}
	}
	
	return result
}