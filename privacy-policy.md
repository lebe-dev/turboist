# Privacy Policy

**Turboist**

**Effective date:** May 24, 2026
**Last updated:** May 24, 2026

## 1. Overview

Turboist ("the Software") is open-source, self-hosted task management software for a single user. The source code is publicly available at https://github.com/lebe-dev/turboist under the MIT License.

This Privacy Policy explains how Turboist handles personal data, with particular attention to data accessed through the optional Google Calendar integration. It is written to comply with the Google API Services User Data Policy, including the Limited Use requirements applicable to Google user data.

## 2. Who we are

Turboist is developed and maintained by individual open-source contributors ("we", "the maintainers"). We are not a company and do not operate a hosted version of the Software.

Because Turboist is self-hosted, **the entity responsible for processing data in any specific instance is the person who deployed and operates that instance** ("the Operator"). For most users, the Operator is the same person as the user — you install and run Turboist for your own use on your own infrastructure.

**The maintainers do not operate any production instance of Turboist, do not receive any user data, and have no ability to access data stored in your instance.**

Contact for privacy questions: **eugene.0x90@gmail.com**

## 3. Scope of this policy

This Privacy Policy describes the data handling behavior of the Turboist Software as designed and distributed by the maintainers. It applies to:

- Anyone who installs and runs Turboist on their own infrastructure.
- Operators evaluating Turboist for deployment.

If you are using a Turboist instance operated by someone else (e.g., a friend or a small group's shared instance), the Operator of that instance is responsible for informing you of their specific data handling practices. This document describes what the Software is capable of doing; the Operator decides how it is deployed.

## 4. What data Turboist processes

Turboist processes the following categories of data, all of which are stored locally on the instance's infrastructure (a SQLite database file by default):

**Account and authentication data.** Username, password hash, JWT signing material, refresh tokens, optional TOTP 2FA secret, and API tokens. These are created locally and never transmitted to the maintainers.

**Task management data.** Tasks, projects, contexts, sections, labels, recurring task rules, and related metadata that you create within the application.

**Google Calendar OAuth tokens** (only if the Google Calendar integration is enabled). See Section 5 below.

**Application logs.** Standard server logs (request identifiers, log levels, anonymized fields) written to the Operator's infrastructure. Logs do not contain Google Calendar event content. Logs remain on the instance and are governed by the Operator's own retention practices.

Turboist does **not** include any analytics, telemetry, advertising trackers, error-reporting services, or third-party SDKs that transmit data outside the instance.

## 5. Google Calendar integration

The Google Calendar integration is **optional** and inactive unless the Operator has explicitly configured it (`GOOGLE_CALENDAR_CLIENT_ID` and `GOOGLE_CALENDAR_CLIENT_SECRET` environment variables) and the user has authorized the connection through Google's OAuth consent screen.

### 5.1 OAuth scope and purpose

Turboist requests a single Google OAuth scope:

- **`https://www.googleapis.com/auth/calendar.readonly`** — read-only access to your Google Calendar events.

**Purpose.** This scope is requested for one purpose only: to display your upcoming Google Calendar events alongside your tasks within the Turboist interface, so that you can plan your day with both task and calendar context visible in one place.

**Limited Use commitment.** Turboist's use of Google user data complies with the [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), including the Limited Use requirements. Specifically:

- Google Calendar data is used **only** to provide the user-facing feature described above.
- Google Calendar data is **not** transferred to third parties.
- Google Calendar data is **not** used for advertising of any kind.
- Google Calendar data is **not** used to develop, train, or improve any general-purpose AI or machine learning models.
- Google Calendar data is **not** read by humans, except (a) by you, the user, in your own Turboist interface; (b) where you give explicit consent for specific data; (c) where required by applicable law; or (d) where strictly necessary for security purposes (e.g., investigating abuse), in which case access would only be possible to the Operator of your instance — never to the maintainers.

### 5.2 What data is accessed

When the integration is active, Turboist reads your Google Calendar events (event title, start and end times, location, description, and similar metadata exposed by the Google Calendar API under the `calendar.readonly` scope) **only at the moment they are needed to render a screen in the application**, by calling the Google Calendar API on demand.

### 5.3 Storage and retention of Google Calendar event data

**Google Calendar event content is not stored on the instance.** Events are fetched from Google's API as needed, held in the application's memory only for the duration of the request, and discarded as soon as the response is returned to the user's browser. There is no local cache, no synchronization to the SQLite database, and no log line that records event content.

If you stop using the application, no Google Calendar event data persists anywhere in Turboist, because none was ever stored.

### 5.4 Storage of OAuth tokens

The OAuth access and refresh tokens issued by Google are stored locally on the instance, in the same SQLite database used for the rest of the application's data. Tokens are **encrypted at rest** using a symmetric key configured by the Operator (`CALENDAR_TOKEN_KEY`, falling back to `JWT_SECRET` if not set).

Tokens are retained until you disconnect the integration or revoke access through your Google Account (see Section 7), at which point they become unusable and are removed from the instance.

### 5.5 No sharing with the maintainers or third parties

The Google Calendar integration runs entirely between your instance and Google's servers. The redirect URI used during the OAuth flow points back to your own instance (`<BASE_URL>/api/v1/calendars/google/callback`). The maintainers receive no callbacks, no tokens, and no event data. No third-party services are involved in the integration.

## 6. Data we (the maintainers) do not collect

The maintainers do not operate any backend, analytics endpoint, license server, update server, or telemetry endpoint that the Software contacts. Running Turboist does not result in any data being transmitted to the maintainers under any circumstance.

If you contact us by email at **eugene.0x90@gmail.com**, we will receive whatever information you choose to send us (e.g., your email address, the contents of your message, attached logs). We use this information only to respond to your inquiry and we do not share it with third parties.

## 7. Your choices and controls

Because Turboist runs on infrastructure you (or your Operator) control, you have full control over your data. Specifically, you can:

**Disconnect the Google Calendar integration at any time.** Use the in-app control to disconnect; the stored OAuth tokens for that integration will be removed from the instance.

**Revoke Turboist's access from Google directly.** Visit https://myaccount.google.com/permissions and remove the OAuth client used by your instance. After revocation, the tokens stored by Turboist will no longer function.

**Delete all data.** Stop the Turboist process and delete the SQLite database file (and its `-wal` / `-shm` companions). This permanently removes all application data, including encrypted OAuth tokens, from the instance.

**Export your data.** Turboist provides a backup/export function that produces a portable copy of your tasks and configuration.

If you are using an instance operated by someone other than yourself, contact your Operator to exercise these rights.

## 8. Security

Turboist applies the following security measures by design:

- Passwords are stored only as cryptographic hashes (never in plaintext).
- Google Calendar OAuth tokens are encrypted at rest using a key controlled by the Operator.
- Authentication uses short-lived JWT access tokens and rotating refresh tokens.
- Optional TOTP-based two-factor authentication is supported.

That said, the overall security of any Turboist deployment depends on the Operator's infrastructure choices (server hardening, TLS configuration, backup encryption, OS patching, etc.). The maintainers cannot guarantee the security of any specific deployment.

If you discover a security issue in the Turboist code itself, please report it via the project's GitHub repository or by email to **eugene.0x90@gmail.com**.

## 9. Children's privacy

Turboist is general-purpose productivity software and is not directed to children under 13 (or the equivalent minimum age in your jurisdiction). The maintainers do not knowingly collect data from children, because the maintainers do not collect any user data at all.

## 10. International data transfers

When you enable the Google Calendar integration, your instance communicates directly with Google's servers, which may be located outside your country. This transfer is governed by Google's own terms and privacy policy. Beyond that, no other international transfers occur as a result of using Turboist itself, because no data leaves your instance.

## 11. Changes to this policy

We may update this Privacy Policy from time to time. The current version is always available at {{ APP_URL }}/privacy-policy. Material changes will be noted by updating the "Last updated" date above.

If a change materially affects how the Software handles user data, we will describe the change in the project's CHANGELOG and in release notes for the version that introduces it.

## 12. Contact

For any questions about this Privacy Policy or about how Turboist handles data, contact:

**eugene.0x90@gmail.com**

Source code and issue tracker: https://github.com/lebe-dev/turboist
