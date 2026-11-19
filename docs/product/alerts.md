# Alerts and Notifications

## When rules run

After every scan, server check and FortiGate read, and after every change to devices, the register or segments, so a warning closes as soon as its cause is gone. Evaluations take a database lock, so two never run at once, and all alerts of one evaluation are written in one statement.

## Rules

"Recent" means seen within two of the longer scan interval, user or server networks (at least 10 minutes; 30 with the defaults).

| Alert | When | Severity |
| --- | --- | --- |
| Unknown device | A real (not private) MAC that isn't approved | Medium on floors, High on server and cloud |
| Private-MAC device | A private MAC that isn't approved, on a segment that watches them (never the gateway) | Medium on floors, High elsewhere |
| Rejected device | A rejected MAC is active | High |
| Unregistered static IP | An address outside the DHCP pool is in use and not registered (never the gateway; never on a DHCP network with no pool set) | Medium |
| Unknown server IP | An unregistered address answers in a server or cloud segment (never the gateway) | High, Critical where the environment is |
| Registered IP misused | A Reserved or Blocked entry, or a reservation, is in use; or a floor entry is used by another MAC than expected | High |
| IP conflict | One IP seen with more than one MAC: for a static address, within the recent window; for an address in a DHCP pool, within the same scan (five minutes), since DHCP hands addresses on all the time | High |
| Server MAC changed | A server answers with another MAC than expected | From its environment, Critical if critical |
| Server down | A monitored server is Down | From its environment, Critical if critical |
| Slow server | A monitored server answers, but slower than the limit in Settings (default 200 ms): the faster of two pings, or the check port's connect time when ping is blocked | Medium |

## One alert per cause

- An alert is open, acknowledged or closed. It closes when its cause is gone. On a segment whose last scan failed, or whose kind isn't scanned automatically, alerts stay open: nobody looked, so the cause may still be there.
- **One open alert per cause.** Device alerts (unknown, private-MAC, rejected) belong to the MAC: when DHCP moves the device to another address, its one alert follows it instead of a new one opening. Other alerts belong to their address.
- A cause that closes and reopens within an hour reopens quietly.
- Closed alerts are kept for a year for the Alert history report.

## Notifications

A bell in the top bar shows each person's notifications and, for those who see the network, how many alerts are open.

- Every new alert is a notification in the app for those who handle alerts. High and Critical ones also go by email when email is switched on in System (off by default, and needs `SMTP_HOST`).
- **More than five new alerts at once become one summary**, so a noisy scan can't flood anyone.
- Also sent: new requests (to those who approve them), request decisions, expiring reservations, and **servers back up**: only when the server is still monitored and answering, not when it was released or its monitoring turned off.
- Each person can switch email off per type. At most 20 emails an hour per person; the rest stay in the app.
- Read notifications are removed after 90 days.
