package main

import (
	"bytes"
	"flag"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
)

var (
	namespace = "archlinux"
	subsystem = "pacman"
	metric    = "upgrade"

	packagesUpgrade = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, metric), "Packages with available upgrade",
		[]string{"name", "installed_version", "upgrade_version"}, nil,
	)
)

type packageUpgrade struct {
	Name             string
	InstalledVersion string
	UpgradeVersion   string
}

type UpgradeCollector struct {
	desc *prometheus.Desc
}

func pacmanQueryUpgrades() (ret []packageUpgrade) {
	out, _ := exec.Command("/usr/bin/pacman", "-Qu").Output()
	buf := bytes.NewBuffer(out)
	for {
		line, err := buf.ReadString('\n')
		if err != nil {
			break
		}
		splitted := strings.Split(line, " ")
		upgrade := packageUpgrade{
			Name:             splitted[0],
			InstalledVersion: splitted[1],
			// Strip trailing \n
			UpgradeVersion: splitted[3][0 : len(splitted[3])-1],
		}
		ret = append(ret, upgrade)
	}
	return
}

func (s *UpgradeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.desc
}

func (s *UpgradeCollector) Collect(ch chan<- prometheus.Metric) {
	for _, upgrade := range pacmanQueryUpgrades() {
		ch <- prometheus.MustNewConstMetric(
			s.desc,
			prometheus.GaugeValue,
			1.0,
			upgrade.Name, upgrade.InstalledVersion, upgrade.UpgradeVersion,
		)
	}
}

func newUpgradeCollector() UpgradeCollector {
	return UpgradeCollector{
		desc: packagesUpgrade,
	}
}

func main() {
	upgradeCollector := newUpgradeCollector()
	prometheus.MustRegister(&upgradeCollector)

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}
