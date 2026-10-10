# Current application reuse and cancellation (#49)

Apply retains one transient application session in its mounted browser view.
Navigation removes the view controls and cancels active work; returning restores completed
analysis and supplied qualification answers. Reloading the application discards
this memory. New Application starts a new session and clears the posting and
analysis. No application artifacts are written to localStorage, cached on the
server, or signed.

## Artifact dependencies

| Artifact | Identity and invalidation | Reuse behavior |
| --- | --- | --- |
| Job understanding | Exact original posting; normalization, structural, repetition, planning and bounds versions; job prompt/model/reasoning/schema; Jev model/rubric/policy | Survives Profile, key, answer and CV-language changes. Revalidated after the server's whole-original Jev gate when supplied to qualification matching or drafting. |
| Qualification analysis | Posting and job policy version; qualification prompt/model/reasoning/schema; approved professional Profile projection, exact document/owner/fact/evidence revisions and context; application session | Unchanged draft retries reopen completed analysis without another qualification request. A changed Profile projection triggers matching with retained job understanding. Exact reference revisions conservatively invalidate matches after a document edit; job understanding remains reusable. |
| Draft | Current Profile and selected fact IDs, posting, job context, application-only confirmations and answers, CV language, and server-owned generation/review policies | Each deliberate draft attempt uses current values and reruns drafting/review. Drafts are never automatically retried or reused after an input change. Existing displayed/accepted materials remain available on failure or cancellation. |

Qualification analysis uses the posting's language and does not consume CV language
or application answers. Those inputs therefore cancel obsolete drafting without
invalidating job understanding or the completed qualification check. API keys are
transport credentials, not artifact identity; changing a key cancels active work
and the next deliberate request uses that key.

The browser qualification version label must change with the matching model,
prompt, schema or policy. The job contract is shared by the browser parser and Go
handlers; its server-owned identity additionally includes preparation versions.
A new deployed browser bundle/reload also discards existing session memory.

## Reuse validation and recovery

Every protected HTTP request classifies the complete, exact original posting with
Jev before consuming an artifact. Client acceptance flags and hashes never skip
that gate. The Go job-context decoder rejects missing/extra fields; preparation
reconstructs occurrence-specific source references and exact original UTF-8
ranges. Protected values and version/source identities must agree with the
server's reconstruction. Invalid reuse returns `job_context_stale` without a
parsing, matching or generation provider call after the field gate.

Checksums establish identity, not authenticity or semantic correctness. Matching
receives the complete original posting to assess the untrusted requirement index;
drafting independently reassesses the job context against the original. Neither
reference presence nor a previous accepted decision establishes support for a
candidate claim or authorizes a changed/blocked posting.

If job extraction completes but qualification matching fails, the error response
may include its validated job context. The browser retains that completed stage;
a deliberate retry repeats matching and the whole-original field gate without
repeating extraction. Invalid/unverifiable artifacts and current materials are
retained for recovery. The user can explicitly discard analysis and analyze again;
there is no automatic repair, retry or model fallback.

## Cancellation

Browser helpers accept an AbortSignal combined with their existing timeout. They
propagate explicit cancellation during both fetching and response consumption.
Changing request inputs, leaving Apply or pressing Cancel aborts the request and
invalidates its request ID. Late responses from transports that ignore AbortSignal
cannot update analysis, materials or navigation. Completed analysis, posting,
answers, existing results and saved Profile survive cancellation.

Go handlers already derive workflow timeouts from the incoming request context and
pass those contexts through preparation, Jev and OpenAI requests. The browser's
abort can reach those contexts where the HTTP host supports disconnection
propagation. Cancellation cannot guarantee that already-started provider work or
charges stop; it prevents obsolete results from applying locally.
