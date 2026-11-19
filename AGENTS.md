# Working on Melon

Read [README.md](README.md) for the layout, how to run it and how to test.

## Every pull request that changes what people see gets a release note

Melon shows a one-time "Nieuw in Melon" dialog listing what changed since
someone last opened it. The notes come from `web/src/content/releases.ts`.

When your change is something a user of the web app, the API, the MCP server
or the calendar feed would notice, add one entry **at the top** of that
list in the same pull request:

```ts
{
  id: 'short-kebab-id', // unique and never reused or changed; it is stored per profile
  date: 'YYYY-MM-DD',   // the day the pull request is opened
  icon: SomeIcon,       // a lucide-react icon that fits, imported at the top of the file
  nl: 'Eén korte zin in het Nederlands.',
  en: 'The same short sentence in English.',
},
```

How to write it:

- One sentence, from the user's point of view: what they can do or see now,
  not how it was built. "Pauzes staan nu in je rooster.", not "Added break
  rows to layoutWeek."
- Dutch first and natural, not a translation of the English. Use "je", like
  the rest of the app. No em dashes.
- Keep it under about 100 characters so it reads as one line on a phone.
- One entry per pull request, even when it changes several small things.
  Pick the change people will notice most.

Skip the note, and say so in the pull request description, when nothing
changes for users: CI, refactors, tests, docs, dependency bumps, and
admin-dashboard-only changes. A bug fix gets a note only when people would
have run into the bug.

Do not change the id of a note that has shipped, and do not remove one:
each profile remembers the id of the last note it saw, and a profile whose
id is gone from the list gets no notes until the next new one. Fixing the
wording is fine.

## Other conventions

- Every user-facing string goes into both `nl` and `en` in
  `web/src/lib/i18n.tsx`. Dutch is the default.
- Before pushing: `cd web && npm run lint && npm run build`, then
  `gofmt -l .` (must print nothing), `go vet ./...` and `go test ./...`.
- When a change is visible, add or update a step in
  `web/scripts/screenshots.cjs` so CI posts a screenshot of it on the pull
  request.
