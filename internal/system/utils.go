// Package system provides OS-level utilities for disk monitoring,
// thermal management, display resolution, and general health checks
// on the Raspberry Pi 5.
package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// HealthStatus represents the current system health snapshot.
type HealthStatus struct {
	DiskUsedPct   float64   `json:"disk_used_pct"`
	DiskFreeBytes uint64    `json:"disk_free_bytes"`
	CPUTempC      float64   `json:"cpu_temp_c"`
	Throttled     bool      `json:"throttled"`
	Timestamp     time.Time `json:"timestamp"`
}

// GetCPUTemp reads the Raspberry Pi thermal zone and returns
// the temperature in degrees Celsius.
func GetCPUTemp() (float64, error) {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err != nil {
		return 0, fmt.Errorf("read cpu temp: %w", err)
	}

	raw := strings.TrimSpace(string(data))
	milliC, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse cpu temp: %w", err)
	}

	return milliC / 1000.0, nil
}

// GetDiskUsage returns the usage percentage and free bytes for
// the filesystem mounted at the given path (default "/").
func GetDiskUsage(path string) (usedPct float64, freeBytes uint64, err error) {
	if path == "" {
		path = "/"
	}

	// Use df to get filesystem stats — portable across Linux distros.
	out, err := exec.Command("df", "--output=pcent,avail", "-B1", path).Output()
	if err != nil {
		return 0, 0, fmt.Errorf("df command failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, 0, fmt.Errorf("unexpected df output")
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("unexpected df fields")
	}

	pctStr := strings.TrimSuffix(fields[0], "%")
	pct, err := strconv.ParseFloat(pctStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse disk pct: %w", err)
	}

	free, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse disk free: %w", err)
	}

	return pct, free, nil
}

// IsThrottled checks the Raspberry Pi's vcgencmd to determine
// if the CPU is currently being throttled due to temperature or
// power supply issues.
func IsThrottled() (bool, error) {
	out, err := exec.Command("vcgencmd", "get_throttled").Output()
	if err != nil {
		return false, fmt.Errorf("vcgencmd failed: %w", err)
	}

	// Output format: throttled=0x0
	parts := strings.SplitN(strings.TrimSpace(string(out)), "=", 2)
	if len(parts) < 2 {
		return false, fmt.Errorf("unexpected vcgencmd output")
	}

	val, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "0x"), 16, 64)
	if err != nil {
		return false, fmt.Errorf("parse throttle value: %w", err)
	}

	return val != 0, nil
}

// SetResolution uses fbset to configure the framebuffer resolution.
// Common values: "1920x1080" or "3840x2160".
func SetResolution(width, height int) error {
	cmd := exec.Command("fbset", "-xres", strconv.Itoa(width), "-yres", strconv.Itoa(height))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("fbset failed: %s: %w", string(out), err)
	}
	log.Printf("[system] resolution set to %dx%d", width, height)
	return nil
}

// RunHealthCheck performs a full system health snapshot.
func RunHealthCheck() HealthStatus {
	status := HealthStatus{
		Timestamp: time.Now(),
	}

	if temp, err := GetCPUTemp(); err == nil {
		status.CPUTempC = temp
	} else {
		log.Printf("[system] health: temp read error: %v", err)
	}

	if pct, free, err := GetDiskUsage("/"); err == nil {
		status.DiskUsedPct = pct
		status.DiskFreeBytes = free
	} else {
		log.Printf("[system] health: disk read error: %v", err)
	}

	if throttled, err := IsThrottled(); err == nil {
		status.Throttled = throttled
	} else {
		log.Printf("[system] health: throttle check error: %v", err)
	}

	log.Printf("[system] health: temp=%.1f°C disk=%.1f%% throttled=%v",
		status.CPUTempC, status.DiskUsedPct, status.Throttled)

	return status
}

// EnsureDir creates a directory and all parents if it does not exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// CleanOldFiles removes files older than maxAge from the given directory.
// Useful for purging stale playlist content.
func CleanOldFiles(dir string, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > maxAge {
			fp := dir + "/" + entry.Name()
			if err := os.Remove(fp); err == nil {
				removed++
				log.Printf("[system] cleaned old file: %s", fp)
			}
		}
	}

	return removed, nil
}
