package main

import (
	"time"

	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/integrations/intune"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/notify"
)

// readerHealth reports, for the daily summary, a FortiGate or Intune read that
// is failing. A reader that works, or was never switched on, says nothing.
func readerHealth(fw *fortigate.Syncer, dir *intune.Syncer) notifications.Health {
	return func() []notify.Row {
		var out []notify.Row
		if s := fw.Snapshot(); s.Error != "" {
			out = append(out, failingRead("FortiGate", "Reading the FortiGate is failing: "+s.Error, s.LastOK, "/system/fortigate"))
		}
		s := dir.Snapshot()
		if s.Error != "" {
			out = append(out, failingRead("Intune", "Reading Microsoft Intune is failing: "+s.Error, s.LastOK, "/system/intune"))
		}
		if s.EntraError != "" {
			out = append(out, failingRead("Entra ID", "Reading Entra ID devices is failing: "+s.EntraError, s.LastOK, "/system/intune"))
		}
		return out
	}
}

func failingRead(chip, text string, lastOK *time.Time, path string) notify.Row {
	meta := "No read has worked yet"
	if lastOK != nil {
		meta = "Last good read " + lastOK.In(time.FixedZone("Sri Lanka", 5*3600+1800)).Format("Mon 2 Jan, 15:04")
	}
	return notify.Row{Tone: "danger", Chip: chip, Text: text, Meta: meta, Path: path}
}
