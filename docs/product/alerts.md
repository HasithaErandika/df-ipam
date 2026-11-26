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

- Every new alert is a notification in the app for those who handle alerts. **More than five new alerts at once become one notice**, so a noisy scan can't flood anyone.
- Also in the app: new requests (to those who approve them), request decisions, expiring reservations, contracts reaching a reminder day, and **servers back up**: only when the server is still monitored and answering, not when it was released or its monitoring turned off.
- Read notifications are removed after 90 days.

## Email

Email is off until an Admin switches it on in **System › Settings › Email** (it needs `SMTP_HOST`). It is kept for what someone must act on, and gathered so it doesn't pile up. Discovery findings are never emailed.

| When | What |
|---|---|
| **At once**, its own email | Critical alerts; a decision on a person's own IP request |
| **Digest**, one email every 30 minutes (15 to 240) | High alerts and new IP requests, gathered per person |
| **Daily summary**, at 08:00 Sri Lanka time (any hour) | Only what needs someone: open alerts (counts and the Critical and High ones), devices and IP requests waiting, DHCP pools and server networks 90% or more used, failing FortiGate, Intune or Entra ID reads and failing scans, network link contracts ending within 30 days, and the person's own reservations about to expire. Each person gets the parts their permissions cover; nothing is sent on a quiet day. |
| **App only** | Medium and Low alerts, servers back up, everything else |

- **Team mailbox (optional).** With an address set (for example a distribution list), alert and request email goes there once instead of to each person, and the daily summary's team parts go there; people then get only their own parts (their reservations, contracts they are responsible for) and their request decisions.
- Each person switches kinds off in **Alerts › Email preferences**: Alerts, New IP requests, Decisions on my IP requests, Daily summary. Only kinds their role can get are listed.
- Emails are HTML with a plain-text copy, in the app's colours, with no images or outside content. Every line links into the app.
- A failed email is tried again 1, 2, 4 and 8 minutes later, then given up. Email isn't sent to deactivated people.
- **Send a test email** in Settings sends an example to the Admin, and to the team mailbox when set.
