package varnishexporter

import (
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	StartParams = &startParams{
		ListenAddress:  ":9131", // Reserved and publicly announced at https://github.com/prometheus/prometheus/wiki/Default-port-allocations
		Path:           "/metrics",
		VarnishstatExe: "varnishstat",
		Params:         &varnishstatParams{},
	}
)

type startParams struct {
	ListenAddress          string
	Path                   string
	HealthPath             string
	VarnishstatExe         string
	VarnishDockerContainer string
	Params                 *varnishstatParams

	Verbose       bool
	ExitOnErrors  bool
	Test          bool
	Raw           bool
	WithGoMetrics bool

	NoExit bool // deprecated
}

type varnishstatParams struct {
	Instance string
	VSM      string
}

func (p *varnishstatParams) isEmpty() bool {
	return p.Instance == "" && p.VSM == ""
}

func (p *varnishstatParams) make() (params []string) {
	// -n
	if p.Instance != "" {
		params = append(params, "-n", p.Instance)
	}
	// -N is not supported by 3.x
	if p.VSM != "" && VarnishVersion.EqualsOrGreater(4, 0) {
		params = append(params, "-N", p.VSM)
	}
	return params
}

// logging

func LogRaw(format string, args ...interface{}) {
	fmt.Printf(format+"\n", args...)
}

func LogTitle(format string, args ...interface{}) {
	LogInfo(format, args...)

	title := strings.Repeat("-", len(fmt.Sprintf(format, args...)))
	if len(title) > 0 {
		LogInfo(title)
	}
}

func LogInfo(format string, args ...interface{}) {
	if StartParams.Raw {
		LogRaw(format, args...)
	} else {
		Logger.Printf(format, args...)
	}
}

func LogWarn(format string, args ...interface{}) {
	format = "[WARN] " + format
	if StartParams.Raw {
		LogRaw(format, args...)
	} else {
		Logger.Printf(format, args...)
	}
}

func LogError(format string, args ...interface{}) {
	format = "[ERROR] " + format
	if StartParams.Raw {
		LogRaw(format, args...)
	} else {
		Logger.Printf(format, args...)
	}
}

func LogFatal(format string, args ...interface{}) {
	format = "[FATAL] " + format
	if StartParams.Raw {
		LogRaw(format, args...)
	} else {
		Logger.Printf(format, args...)
	}
	os.Exit(1)
}

func LogFatalError(err error) {
	if err != nil {
		LogFatal(err.Error())
	}
}

// strings

type caseSensitivity int

const (
	caseSensitive   caseSensitivity = 0
	caseInsensitive caseSensitivity = 1
)

func startsWith(str, prefix string, cs caseSensitivity) bool {
	if cs == caseSensitive {
		return strings.HasPrefix(str, prefix)
	}
	return strings.HasPrefix(strings.ToLower(str), strings.ToLower(prefix))
}

func startsWithAny(str string, prefixes []string, cs caseSensitivity) bool {
	for _, prefix := range prefixes {
		if startsWith(str, prefix, cs) {
			return true
		}
	}
	return false
}

func endsWith(str, postfix string, cs caseSensitivity) bool {
	if cs == caseSensitive {
		return strings.HasSuffix(str, postfix)
	}
	return strings.HasSuffix(strings.ToLower(str), strings.ToLower(postfix))
}

func endsWithAny(str string, postfixes []string, cs caseSensitivity) bool {
	for _, postfix := range postfixes {
		if endsWith(str, postfix, cs) {
			return true
		}
	}
	return false
}

// file

// Returns if file/dir in path exists.
func fileExists(path string) bool {
	if len(path) == 0 {
		return false
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// data

func stringProperty(data map[string]interface{}, key string) (string, error) {
	if value, ok := data[key]; ok {
		if vStr, ok := value.(string); ok {
			return vStr, nil
		} else {
			return "", fmt.Errorf("%s is not a string", key)
		}
	}
	return "", nil
}
