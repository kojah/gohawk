# channelsafety design notes

The public page is [channelsafety](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Complete concurrency summaries expose closes and sends inside synchronous
helpers, including imported helpers. A close in one call can therefore be
matched to a later direct send or helper call on the exact same channel.
Diagnostics point to the caller's send (or sending call), with a related
location for the close (or closing call).

This supplements the existing local flow proof; it does not require the
caller itself to be straight-line. The summary path declines branching or
opaque helper bodies and uncertain identities. Deferred and asynchronous
calls are not treated as executing at their registration or launch site.
Send-after-close comparisons do not cross loop backedges or compare events
within one call.
