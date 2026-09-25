# deferinloop design notes

The public page is [deferinloop](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Storing the resource in a containing aggregate makes its iteration-local
lifetime unknown, including a collection populated before the defer. Such
collections may deliberately keep resources for use after the loop. This
boundary does not prove the collection is eventually closed.
