package prefilter

import (
	"regexp"
	"strings"
)

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionTag   Decision = "tag"
	DecisionBlock Decision = "block"
)

type Result struct {
	Decision Decision
	Reason   string
	Input    string
	Tags     []string
}

type Filter struct {
	rolePrefix *regexp.Regexp
}

func New() Filter {
	return Filter{
		rolePrefix: regexp.MustCompile(`(?i)^\s*[a-z][a-z0-9 -]{1,40}\s*:\s*`),
	}
}

func (f Filter) Check(input string) Result {
	lower := strings.ToLower(input)
	if strings.Contains(lower, "csam") || strings.Contains(lower, "[slur]") || strings.Contains(lower, "personal attack:") {
		return Result{Decision: DecisionBlock, Reason: "abuse fixture", Input: input}
	}
	if strings.Contains(lower, "ignore previous instructions") || strings.Contains(lower, "disregard your prior instructions") {
		return Result{Decision: DecisionTag, Reason: "prompt injection phrase", Input: input, Tags: []string{"prompt_injection"}}
	}
	if f.rolePrefix.MatchString(input) {
		return Result{Decision: DecisionTag, Reason: "role-prefix attempt", Input: input, Tags: []string{"role_prefix"}}
	}
	return Result{Decision: DecisionAllow, Input: input}
}
