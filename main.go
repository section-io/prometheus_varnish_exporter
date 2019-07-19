package main // import "github.com/jonnenauha/prometheus_varnish_exporter"

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jonnenauha/prometheus_varnish_exporter/varnishexporter"
)

var (
	ApplicationName = "prometheus_varnish_exporter"
	Version         string
	VersionHash     string
	VersionDate     string
)

func init() {
	// prometheus conventions
	flag.StringVar(&varnishexporter.StartParams.ListenAddress, "web.listen-address", varnishexporter.StartParams.ListenAddress, "Address on which to expose metrics and web interface.")
	flag.StringVar(&varnishexporter.StartParams.Path, "web.telemetry-path", varnishexporter.StartParams.Path, "Path under which to expose metrics.")
	flag.StringVar(&varnishexporter.StartParams.HealthPath, "web.health-path", varnishexporter.StartParams.HealthPath, "Path under which to expose healthcheck. Disabled unless configured.")

	// varnish
	flag.StringVar(&varnishexporter.StartParams.VarnishstatExe, "varnishstat-path", varnishexporter.StartParams.VarnishstatExe, "Path to varnishstat.")
	flag.StringVar(&varnishexporter.StartParams.Params.Instance, "n", varnishexporter.StartParams.Params.Instance, "varnishstat -n value.")
	flag.StringVar(&varnishexporter.StartParams.Params.VSM, "N", varnishexporter.StartParams.Params.VSM, "varnishstat -N value.")

	// docker
	flag.StringVar(&varnishexporter.StartParams.VarnishDockerContainer, "docker-container-name", varnishexporter.StartParams.VarnishDockerContainer, "Docker container name to exec varnishstat in.")

	// modes
	version := false
	flag.BoolVar(&version, "version", version, "Print version and exit")
	flag.BoolVar(&varnishexporter.StartParams.ExitOnErrors, "exit-on-errors", varnishexporter.StartParams.ExitOnErrors, "Exit process on scrape errors.")
	flag.BoolVar(&varnishexporter.StartParams.Verbose, "verbose", varnishexporter.StartParams.Verbose, "Verbose varnishexporter.Logging.")
	flag.BoolVar(&varnishexporter.StartParams.Test, "test", varnishexporter.StartParams.Test, "Test varnishstat availability, prints available metrics and exits.")
	flag.BoolVar(&varnishexporter.StartParams.Raw, "raw", varnishexporter.StartParams.Test, "Raw stdout varnishexporter.Logging without timestamps.")
	flag.BoolVar(&varnishexporter.StartParams.WithGoMetrics, "with-go-metrics", varnishexporter.StartParams.WithGoMetrics, "Export go runtime and http handler metrics")

	// deprecated
	flag.BoolVar(&varnishexporter.StartParams.NoExit, "no-exit", varnishexporter.StartParams.NoExit, "Deprecated: see -exit-on-errors")

	flag.Parse()

	if version {
		fmt.Printf("%s %s\n", ApplicationName, getVersion(true))
		os.Exit(0)
	}

	if len(varnishexporter.StartParams.Path) == 0 || varnishexporter.StartParams.Path[0] != '/' {
		varnishexporter.LogFatal("-web.telemetry-path cannot be empty and must start with a slash '/', given %q", varnishexporter.StartParams.Path)
	}
	if len(varnishexporter.StartParams.HealthPath) != 0 && varnishexporter.StartParams.HealthPath[0] != '/' {
		varnishexporter.LogFatal("-web.health-path must start with a slash '/' if configured, given %q", varnishexporter.StartParams.HealthPath)
	}
	if varnishexporter.StartParams.Path == varnishexporter.StartParams.HealthPath {
		varnishexporter.LogFatal("-web.telemetry-path and -web.health-path cannot have same value")
	}

	// Don't varnishexporter.Log warning on !noExit as that would spam for the formed default value.
	if varnishexporter.StartParams.NoExit {
		varnishexporter.LogWarn("-no-exit is deprecated. As of v1.5 it is the default behavior not to exit process on scrape errors. You can remove this parameter.")
	}

	// Test run or user explicitly wants to exit on any scrape errors during runtime.
	varnishexporter.ExitHandler.ExitOnError = varnishexporter.StartParams.Test == true || varnishexporter.StartParams.ExitOnErrors == true
}

func main() {
	if b, err := json.MarshalIndent(varnishexporter.StartParams, "", "  "); err == nil {
		varnishexporter.LogInfo("%s %s %s", ApplicationName, getVersion(false), b)
	} else {
		varnishexporter.LogFatal(err.Error())
	}

	// Initialize
	if err := varnishexporter.VarnishVersion.Initialize(); err != nil {
		varnishexporter.ExitHandler.Errorf("Varnish version initialize failed: %s", err.Error())
	}
	if varnishexporter.VarnishVersion.Valid() {
		varnishexporter.LogInfo("Found varnishstat %s", varnishexporter.VarnishVersion)
		if err := varnishexporter.PrometheusExporter.Initialize(); err != nil {
			varnishexporter.LogFatal("Prometheus exporter initialize failed: %s", err.Error())
		}
	}

	// Test to verify everything is ok before starting the server
	{
		done := make(chan bool)
		metrics := make(chan prometheus.Metric)
		go func() {
			for m := range metrics {
				if varnishexporter.StartParams.Test {
					varnishexporter.LogInfo("%s", m.Desc())
				}
			}
			done <- true
		}()
		tStart := time.Now()
		buf, err := varnishexporter.ScrapeVarnish(metrics)
		close(metrics)
		<-done

		if err == nil {
			varnishexporter.LogInfo("Test scrape done in %s", time.Now().Sub(tStart))
			varnishexporter.LogRaw("")
		} else {
			if len(buf) > 0 {
				varnishexporter.LogRaw("\n%s", buf)
			}
			varnishexporter.ExitHandler.Errorf("Startup test: %s", err.Error())
		}
	}
	if varnishexporter.StartParams.Test {
		return
	}

	// Start serving
	varnishexporter.LogInfo("Server starting on %s with metrics path %s", varnishexporter.StartParams.ListenAddress, varnishexporter.StartParams.Path)

	if !varnishexporter.StartParams.WithGoMetrics {
		registry := prometheus.NewRegistry()
		if err := registry.Register(varnishexporter.PrometheusExporter); err != nil {
			varnishexporter.LogFatal("registry.Register failed: %s", err.Error())
		}
		handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			ErrorLog: varnishexporter.Logger,
		})
		http.Handle(varnishexporter.StartParams.Path, handler)
	} else {
		prometheus.MustRegister(varnishexporter.PrometheusExporter)
		http.Handle(varnishexporter.StartParams.Path, promhttp.Handler())
	}

	if varnishexporter.StartParams.Path != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`<html>
    <head><title>Varnish Exporter</title></head>
    <body>
        <h1>Varnish Exporter</h1>
    	<p><a href="` + varnishexporter.StartParams.Path + `">Metrics</a></p>
    </body>
</html>`))
		})
	}
	if varnishexporter.StartParams.HealthPath != "" {
		http.HandleFunc(varnishexporter.StartParams.HealthPath, func(w http.ResponseWriter, r *http.Request) {
			// As noted in the "up" metric, needs some way to determine if everything is actually Ok.
			// For now, this just lets us check that we're accepting connections
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "Ok")
		})
	}
	varnishexporter.LogFatalError(http.ListenAndServe(varnishexporter.StartParams.ListenAddress, nil))
}

func getVersion(date bool) (version string) {
	if Version == "" {
		return "dev"
	}
	version = fmt.Sprintf("v%s (%s)", Version, VersionHash)
	if date {
		version += " " + VersionDate
	}
	return version
}
