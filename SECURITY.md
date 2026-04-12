# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in KodaCode, please report it responsibly.

**Do not open a public issue.** Instead, email **sammy.ageil@outlook.com** with:

- A description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

You should receive an acknowledgment within 48 hours. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Scope

Security issues in the following areas are in scope:

- **Sandbox escapes** — any way to read, write, or execute outside the project directory without explicit permission
- **Permission bypasses** — circumventing allow/ask/deny rules
- **Injection** — command injection via tool arguments, path traversal, or config parsing
- **Credential exposure** — API keys or tokens leaked in logs, error messages, or session data
- **Denial of service** — inputs that crash or hang KodaCode

## Out of Scope

- Vulnerabilities in upstream dependencies (report to the upstream project directly)
- Issues that require physical access to the machine
- Social engineering

## Supported Versions

Security fixes are applied to the latest release only. We recommend always running the latest version.
