package testrunner

import "regexp"

func compileRegex(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}
