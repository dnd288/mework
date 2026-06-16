package webhook

import (
	"strings"
)

// ParseTrigger parses the trigger grammar from a comment body:
// "@mework [profile-name] [workflow-name] [free instructions]"
// It returns profile, workflow, instructions, and a boolean indicating if a trigger was parsed.
func ParseTrigger(body string) (profile, workflow, instructions string, ok bool) {
	// Find @mework with word boundary
	idx := -1
	if strings.HasPrefix(body, "@mework") {
		idx = 0
	} else {
		idx = strings.Index(body, " @mework")
		if idx != -1 {
			idx++ // Adjust for the space
		} else {
			idx = strings.Index(body, "\n@mework")
			if idx != -1 {
				idx++ // Adjust for the newline
			}
		}
	}

	if idx == -1 {
		return "", "", "", false
	}

	remaining := body[idx+len("@mework"):]
	words := strings.Fields(remaining)
	if len(words) == 0 {
		return "", "", "", false
	}

	profile = words[0]

	if len(words) >= 2 {
		secondWord := words[1]
		if isRecognizedWorkflow(secondWord) {
			workflow = secondWord
			// Find index of the workflow word in remaining to slice the rest properly
			pos := strings.Index(remaining, secondWord)
			if pos != -1 {
				instRaw := remaining[pos+len(secondWord):]
				instructions = strings.TrimSpace(instRaw)
			}
		} else {
			workflow = ""
			// Find index of the profile word in remaining to slice the rest properly
			pos := strings.Index(remaining, profile)
			if pos != -1 {
				instRaw := remaining[pos+len(profile):]
				instructions = strings.TrimSpace(instRaw)
			}
		}
	} else {
		workflow = ""
		instructions = ""
	}

	return profile, workflow, instructions, true
}

func isRecognizedWorkflow(w string) bool {
	switch strings.ToLower(w) {
	case "plan", "cook", "test", "review", "ship", "journal":
		return true
	default:
		return false
	}
}
