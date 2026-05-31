# Terms of Service

**Turboist**

**Effective date:** May 24, 2026
**Last updated:** May 24, 2026

## 1. About these Terms

Turboist ("the Software") is open-source software for personal task management, distributed under the MIT License. The source code is publicly available at https://github.com/lebe-dev/turboist.

These Terms of Service ("Terms") govern your use of the Software when you download, install, or run it on infrastructure you control ("Self-Hosted Use").

By installing, accessing, or using the Software, you agree to these Terms. If you do not agree, do not use the Software.

## 2. Definitions

- **"We", "us", "the maintainers"** — the individual contributors who develop and publish the Software at https://github.com/lebe-dev/turboist. We are not a company and do not provide a commercial service.
- **"You", "the user"** — the natural person installing, operating, or using the Software.
- **"Self-Hosted Instance"** — any deployment of the Software on infrastructure controlled by you or a third party other than us.
- **"Operator"** — the person or entity that runs a specific instance of the Software and controls the server, storage, and configuration for that instance. For Self-Hosted Instances, the Operator is you (or whoever deployed it).

## 3. Nature of the Software

Turboist is single-user, solo task management software intended to be run by one user for their own use. It is not a multi-tenant service and we do not operate a hosted version of it. The Software stores data locally on the infrastructure on which it is deployed.

## 4. Self-Hosted Use

### 4.1 Your role as Operator

When you deploy a Self-Hosted Instance, **you are the Operator** of that instance. This means:

- You are solely responsible for the installation, configuration, security, backups, updates, and lawful operation of your instance.
- You control where data is stored and who can access it.
- We have no access to, visibility into, or control over your instance or the data within it.

### 4.2 Third-party integrations

The Software includes an optional integration with Google Calendar. When you enable it:

- You register your own OAuth credentials in your own Google Cloud project, or you use credentials provided by the Operator of your instance.
- The OAuth authorization is granted by you directly to your instance. Data retrieved from Google Calendar is stored only on your instance.
- The Software requests read-only access to your Google Calendar (the `calendar.readonly` scope) and does not modify your calendar data.
- Your use of Google Calendar is also subject to Google's Terms of Service and Privacy Policy. We are not a party to that relationship.

### 4.3 No service obligations

For Self-Hosted Use, we provide no uptime, availability, support, security, or update guarantees of any kind. The Software is provided "as is" under the terms of the MIT License.

## 5. Your data

For Self-Hosted Instances, all data — including any data retrieved from Google Calendar — is stored exclusively on the infrastructure operated by you or your Operator. We do not collect, receive, or have access to this data. All data handling is determined by the Operator of the instance.

Further details about what data the Software processes and how Google Calendar data is handled are described in our Privacy Policy at {{ APP_URL }}/privacy-policy.

## 6. Intellectual property

The Software is licensed under the MIT License. Your rights to use, modify, and distribute the Software are governed by that license, the full text of which is available at https://github.com/lebe-dev/turboist/blob/main/LICENSE. Nothing in these Terms grants you any rights beyond those granted by the license.

## 7. Disclaimer of warranties

THE SOFTWARE IS PROVIDED "AS IS" AND "AS AVAILABLE", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, NON-INFRINGEMENT, OR THAT THE SOFTWARE WILL BE ERROR-FREE, SECURE, OR UNINTERRUPTED. YOU USE THE SOFTWARE AT YOUR OWN RISK.

## 8. Limitation of liability

TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, IN NO EVENT WILL THE MAINTAINERS BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL, OR PUNITIVE DAMAGES, OR ANY LOSS OF PROFITS, DATA, USE, OR GOODWILL, ARISING OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THESE TERMS, WHETHER BASED ON CONTRACT, TORT, OR ANY OTHER LEGAL THEORY, AND WHETHER OR NOT WE HAVE BEEN ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

WHERE LIABILITY CANNOT BE EXCLUDED UNDER APPLICABLE LAW, OUR TOTAL LIABILITY IS LIMITED TO THE GREATER OF (A) THE AMOUNT YOU PAID US IN THE PAST TWELVE MONTHS (WHICH FOR FREE USE WILL BE ZERO) AND (B) USD 50.

## 9. Indemnification

If your operation of a Self-Hosted Instance causes a claim, demand, or expense against us, you agree to indemnify and hold us harmless from such claim to the extent it arises from your operation of the instance, your configuration choices, your use of third-party integrations, or your processing of personal data.

## 10. Termination

You may stop using the Software at any time by uninstalling it. The provisions of Sections 6–9 and 12 survive termination.

## 11. Changes to these Terms

We may update these Terms from time to time. The current version is always available at {{ APP_URL }}/terms-of-service. Material changes will be noted by updating the "Last updated" date above. Your continued use of the Software after changes take effect constitutes acceptance of the updated Terms.

## 12. Contact

Questions about these Terms can be sent to: **eugene.0x90@gmail.com**

Source code and issue tracker: https://github.com/lebe-dev/turboist
