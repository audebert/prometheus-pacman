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

	"github.com/golang/glog"
)

var (
	addr = flag.String("listen-address", ":9101", "The address to listen on for HTTP requests.")
)

var (
	namespace = "archlinux"
	subsystem = "pacman"

	installedMetric = "installed"
	ignoreMetric    = "ignored"
	upgradesMetric  = "upgrade"

	packagesInstalledDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, installedMetric), "Installed packages",
		[]string{"package_name", "installed_version"}, nil,
	)
	packagesUpgradeDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, upgradesMetric), "Packages with available upgrade",
		[]string{"package_name", "installed_version", "upgrade_version"}, nil,
	)
	packagesIgnoredDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, subsystem, ignoreMetric), "Packages with ignored upgrade",
		[]string{"package_name", "installed_version", "upgrade_version"}, nil,
	)
)

type packageInstalled struct {
	PackageName      string
	InstalledVersion string
}

type upgradeAvailable struct {
	PackageName      string
	InstalledVersion string
	UpgradeVersion   string
}

type upgradeIgnored struct {
	PackageName      string
	InstalledVersion string
	UpgradeVersion   string
}

type PacmanCollector struct {
	installed *prometheus.Desc
	upgrades  *prometheus.Desc
	ignored   *prometheus.Desc
}

func pacmanQueryInstalled() []packageInstalled {
	var installed []packageInstalled

	out, _ := exec.Command("/usr/bin/pacman", "-Q").Output()
	buf := bytes.NewBuffer(out)
	for {
		line, err := buf.ReadString('\n')
		if err != nil {
			break
		}
		// Strip \n
		line = line[0 : len(line)-1]
		fields := strings.Split(line, " ")
		if len(fields) != 2 {
			glog.Warningf("Wrong number of items in %s", line)
			continue
		}
		pkg := packageInstalled{
			PackageName:      fields[0],
			InstalledVersion: fields[1],
		}
		installed = append(installed, pkg)
	}

	return installed
}

func pacmanQueryUpgrades() ([]upgradeAvailable, []upgradeIgnored) {
	var available []upgradeAvailable
	var ignored []upgradeIgnored

	out, _ := exec.Command("/usr/bin/pacman", "-Qu").Output()
	buf := bytes.NewBuffer(out)

	for {
		line, err := buf.ReadString('\n')
		if err != nil {
			break
		}
		// Strip \n
		line = line[0 : len(line)-1]
		fields := strings.Split(line, " ")
		if fields[2] != "->" {
			glog.Warningf("Expected \"->\" but got \"%s\" in: %s", fields[2], line)
			continue
		}

		upgrade := upgradeAvailable{
			PackageName:      fields[0],
			InstalledVersion: fields[1],
			// Strip trailing \n
			UpgradeVersion: fields[3],
		}
		available = append(available, upgrade)

		if len(fields) == 5 {
			if fields[4] == "[ignored]" {
				ignore := upgradeIgnored{
					PackageName:      fields[0],
					InstalledVersion: fields[1],
					UpgradeVersion:   fields[3],
				}
				ignored = append(ignored, ignore)
			} else {
				glog.Warningf("Unknown format: %s", line)
			}
		}
	}
	return available, ignored
}

func (s *PacmanCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- s.installed
	ch <- s.upgrades
	ch <- s.ignored
}

func (s *PacmanCollector) Collect(ch chan<- prometheus.Metric) {
	for _, installed := range pacmanQueryInstalled() {
		ch <- prometheus.MustNewConstMetric(
			s.installed,
			prometheus.GaugeValue,
			1.0,
			installed.PackageName, installed.InstalledVersion,
		)
	}

	upgrades, ignored := pacmanQueryUpgrades()
	for _, upgrade := range upgrades {
		ch <- prometheus.MustNewConstMetric(
			s.upgrades,
			prometheus.GaugeValue,
			1.0,
			upgrade.PackageName, upgrade.InstalledVersion, upgrade.UpgradeVersion,
		)
	}
	for _, pkg := range ignored {
		ch <- prometheus.MustNewConstMetric(
			s.ignored,
			prometheus.GaugeValue,
			1.0,
			pkg.PackageName, pkg.InstalledVersion, pkg.UpgradeVersion,
		)
	}
}

func newPacmanCollector() PacmanCollector {
	return PacmanCollector{
		installed: packagesInstalledDesc,
		upgrades:  packagesUpgradeDesc,
		ignored:   packagesIgnoredDesc,
	}
}

func main() {
	flag.Parse()

	upgradeCollector := newPacmanCollector()
	prometheus.MustRegister(&upgradeCollector)

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*addr, nil))
}
