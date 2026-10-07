# Apply checklist transitions (#24)

Transition rules recorded before implementation:

- API key, Profile, and posting are pending until present/sufficient; then ready (green). A configured key is only ready to attempt a request, never provider-validated.
- Qualification checking starts neutral/processing. Successful checking becomes complete (green); gaps leave a neutral confirmation-needed state until the user continues. Draft stays pending during checking/confirmation.
- Draft starts neutral/processing only when qualification checking permits it or the user explicitly continues from confirmation. Success completes it; a request error fails that step (red).
- A key rejection also fails the key prerequisite. An input rejection marks Profile and posting as needing review because the API does not identify which input failed. Transport, timeout, rate-limit, outage, and invalid-output failures affect the request step, not otherwise ready inputs.
- Missing inputs are pending/action-needed, never failures. Any relevant key/Profile/posting edit clears earlier request results/errors and confirmations. A deliberate retry clears failures before requesting. Confirmation edits clear draft failures.
- Dismissing confirmation returns request steps to pending. Leaving Apply invalidates in-flight results and clears the global processing flag; stale responses cannot navigate or overwrite newer results. Editing during a request also invalidates it.
- Status labels and details convey the state without color. Processing/confirmation changes use polite live announcements; errors use an alert. Details wrap at narrow widths. The app currently supports a single light theme.

Verification uses the rendered Apply UI and controlled `/api/qualification-gaps` and `/api/application-draft` responses, with synthetic data and no provider calls.

## Verification (2026-10-07)

`npm run test:apply-checklist` passed for English and Portuguese at 1440px and 390px. The browser check covers missing prerequisites, ready key wording, qualification/draft processing, qualification confirmation, key/input/rate-limit/outage/timeout/invalid-output errors on each request step, a browser timeout rejection, confirmation edit recovery, retry clearing, edits/navigation during a request, and repeated successful generation. It asserts readable status colors and no horizontal page overflow. Screenshots were captured and the Portuguese 390px layout was visually inspected. Synthetic API responses were used; this does not verify a live provider or deployment.

`tsc --noEmit`, `npm test` (all three frontend suites), `vite build`, and `GOCACHE=/tmp/careeros-go-cache go test ./...` passed. The existing Vite native-config warning remains.
