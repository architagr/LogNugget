# Security Policy

## Supported Versions

Supported versions: **v1.x** and **v2.x** (latest tag on `main`). Security fixes are accepted for both active lines.

## Reporting a Vulnerability

Please do **not** open a public issue for suspected vulnerabilities.

Use GitHub's private vulnerability reporting flow instead:

1. Open this repository's **Security** tab.
2. Choose **Report a vulnerability**.
3. Include the details listed below.

If private vulnerability reporting is not yet visible, email the maintainer at architagr@gmail.com and ask for a private reporting channel before sharing exploit details.

## What to Include

Please include enough information for maintainers to reproduce and triage safely:

- LogNugget version, commit, or tag.
- Go version and operating system.
- Affected package or API path.
- Description of the issue and expected impact.
- Minimal reproduction steps or test case.
- Any logs, panic output, or relevant configuration.
- Whether the issue is already public or privately held.

## Response Targets

Maintainers aim to:

- Acknowledge valid-looking reports within **72 hours**.
- Provide an initial severity/impact assessment within **7 days**.
- Patch or mitigate critical issues within **14 days** where feasible.

Timelines may vary with severity, reproduction quality, maintainer availability, and whether the fix requires coordinated downstream release work.

## Coordinated Disclosure

Please keep vulnerability details private until maintainers have had a reasonable chance to investigate and release a fix. Reporters will be notified before public disclosure or advisory publication whenever practical.

## Safe Research Boundaries

Please use local tests or your own deployments. Do not attack third-party systems, exfiltrate data, perform denial-of-service testing, or access accounts/data that are not yours.
