# Plan: chat in Melon

Status: **parked, not a priority.** This is the plan for when it is.

## Why

Students at JFC use Google Chat and nobody likes it. Melon already knows who
everyone is and which lesson groups they are in, so it can do something Google
Chat can't: class chats that set themselves up.

## What's in it (v1)

- **Direct messages** between two students at the same school.
- **Lesson-group chats** that exist on their own: every lesson group (e.g.
  `5vwo.wiskB2`) gets a chat with the Melon users who have that group in their
  schedule. No creating, no inviting. You join when the group shows up in your
  schedule and leave when it stops doing so.
- **Text only.** No images, files, voice or links previews. Images are where
  the worst abuse happens (nudes, fake pictures of classmates), and v1 does not
  take that on.
- **Students only.** Accounts with role `student`. No teachers, no parents.
- **Off by default, per school.** Chat only runs at a school that has someone
  moderating it. JFC first.

Not in v1: editing messages, reactions, read receipts, typing indicators,
custom groups, push notifications. Each of these is easy later and none is
why people would switch.

## Who you are in chat

A chat identity is a **Zermelo account**, not a Melon profile. A profile can
hold several accounts (and parent accounts hold children), so:

- Only accounts with `role = 'student'` and no `owner_code` can chat. An
  account a parent linked never gets to talk as the child.
- If one profile holds two student accounts, it picks which one it is chatting
  as, like it picks which schedule it is looking at.
- Name shown is the Zermelo display name. Nicknames stay private.

## Data (SQLite, `internal/store/chat.go`)

```sql
CREATE TABLE chat_schools (           -- schools where chat is on
  school TEXT PRIMARY KEY, enabled_at INTEGER NOT NULL, enabled_by TEXT NOT NULL
);
CREATE TABLE chat_rooms (
  id TEXT PRIMARY KEY,                -- room_...
  school TEXT NOT NULL,
  kind TEXT NOT NULL,                 -- 'dm' | 'group'
  key TEXT NOT NULL,                  -- dm: "<userA>|<userB>" sorted; group: Zermelo group code
  title TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  UNIQUE (school, kind, key)
);
CREATE TABLE chat_members (
  room_id TEXT NOT NULL REFERENCES chat_rooms(id) ON DELETE CASCADE,
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  last_read INTEGER NOT NULL DEFAULT 0,   -- message id
  muted INTEGER NOT NULL DEFAULT 0,
  seen_at INTEGER NOT NULL,               -- last time the schedule confirmed membership
  PRIMARY KEY (room_id, account_id)
);
CREATE TABLE chat_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,   -- ordering + cursor
  room_id TEXT NOT NULL REFERENCES chat_rooms(id) ON DELETE CASCADE,
  account_id TEXT REFERENCES accounts(id) ON DELETE SET NULL,
  body TEXT NOT NULL,                     -- max 2000 chars
  created_at INTEGER NOT NULL,
  hidden TEXT NOT NULL DEFAULT '',        -- '' | 'ai' | 'admin' | 'author'
  ai_score REAL                           -- highest moderation score, if checked
);
CREATE INDEX chat_messages_room ON chat_messages(room_id, id);
CREATE TABLE chat_blocks (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  blocked_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (account_id, blocked_id)
);
CREATE TABLE chat_reports (
  id TEXT PRIMARY KEY,
  message_id INTEGER NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
  reporter_id TEXT REFERENCES accounts(id) ON DELETE SET NULL, -- NULL = the AI check
  reason TEXT NOT NULL,                   -- 'bullying' | 'sexual' | 'threat' | 'spam' | 'other' | 'ai:<category>'
  note TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  resolved_at INTEGER, resolved_by TEXT, outcome TEXT
);
CREATE TABLE chat_sanctions (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,                     -- 'mute' | 'ban'
  until INTEGER,                          -- NULL = permanent
  reason TEXT NOT NULL, by TEXT NOT NULL, created_at INTEGER NOT NULL
);
```

Messages are not encrypted at rest. Moderation needs to read reported ones, and
end-to-end encryption would make reports unverifiable. Say so in the privacy
notice.

**Retention:** messages are deleted after 1 year, and at the end of the school
year for group chats (the groups stop existing anyway). Deleting a profile
deletes its accounts, which nulls the author on its messages; the text goes
too (set `body = ''`, `hidden = 'author'`).

## Group membership

Membership comes from schedules Melon already fetches, so no extra Zermelo
calls:

1. When `core` builds a week for a student account at a chat-enabled school,
   it collects the distinct `groups` of its lessons.
2. It upserts a `chat_rooms` row per group and a `chat_members` row with
   `seen_at = now`.
3. A daily job removes members whose `seen_at` is older than 21 days (covers
   holidays) and rooms with no members left.

Only groups with at least 3 Melon users show up in the chat list, so a "group
chat" is never secretly a DM.

## Live updates

Server-Sent Events, stdlib only.

- `GET /api/v1/chat/stream?as=acc_...` keeps one connection per open tab.
  Events: `message`, `hidden`, `read`.
- An in-memory hub (`internal/core/chathub.go`) maps account id to open
  streams and fans out new messages. One process, so no Redis.
- Heartbeat comment every 25 s so proxies don't close it.
- The client reconnects with `Last-Event-ID` (the message id) and gets what it
  missed from the database.
- If Melon ever runs as more than one process, swap the hub for SQLite polling
  or Postgres `LISTEN/NOTIFY`. Not before.

## API (`/api/v1/chat`, session cookie only)

Not on API keys or MCP in v1. A bot that can send messages as you is a spam
cannon. Reading could come later.

| Method | Path | Does |
|---|---|---|
| GET | `/chat/rooms?as=` | Rooms with last message and unread count |
| POST | `/chat/dm?as=` `{to}` | Find or create the DM with another student |
| GET | `/chat/rooms/{id}/messages?as=&before=` | 50 messages, newest first |
| POST | `/chat/rooms/{id}/messages?as=` `{body}` | Send |
| POST | `/chat/rooms/{id}/read?as=` `{upTo}` | Mark read |
| DELETE | `/chat/messages/{id}?as=` | Delete your own message |
| POST | `/chat/messages/{id}/report?as=` `{reason, note}` | Report |
| POST/DELETE | `/chat/blocks/{accountId}?as=` | Block / unblock |
| GET | `/chat/stream?as=` | SSE |

Every handler checks: account belongs to this profile, role is student, chat is
on for its school, no active ban, and (for sending) no mute.

## Moderation

Nobody reads chats unless something is reported or flagged.

**Users**

- **Block**: they can't DM you, you don't see their messages in groups (shown
  as "Bericht van geblokkeerde persoon"), and they don't learn they're blocked.
- **Report** a message with a reason. The reporter sees it disappear for them
  right away.
- Leave or mute a group chat.

**Automatic**

- **Rate limit**: 20 messages per minute per account, 5 new DMs per hour.
  Over the limit means a short cooldown, not an error page.
- **Word list** of slurs and the worst insults (NL + EN), checked on send.
  A hit is rejected with a friendly message. Kept short on purpose: it catches
  the obvious, the AI check catches the rest.
- **AI check** via the Hack Club AI moderations endpoint
  (`POST https://ai.hackclub.com/proxy/v1/moderations`,
  `omni-moderation-latest`):
  - Runs **after** the message is sent, in a background queue that batches
    messages. Sending never waits on it, and the chat keeps working when the
    API is down (unchecked messages are retried later).
  - `flagged` creates a report with `reporter_id = NULL` and reason
    `ai:<category>`.
  - A very high score on `sexual/minors`, `self-harm/intent`, `violence` or
    `harassment/threatening` (start at 0.9, tune later) also hides the message
    for everyone except its author until a moderator looks.
  - It never bans or mutes anyone on its own. Dutch slang ("ik maak je af")
    trips it.
  - Turned on with `MELON_MODERATION_KEY`; without it, everything else still
    works.
  - Before turning it on: check the Hack Club AI rules and rate limits, and
    name it in the privacy notice (messages go to Hack Club and from there to
    OpenAI).

**Moderators**

- New role next to `MELON_ADMINS`: chat moderators **per school**, set by an
  admin in the dashboard. A moderator only sees their own school.
- New *Chat* tab in the admin dashboard:
  - Report queue: the message, 5 messages before and after it in that room,
    the reporter (hidden from the reported person), the AI scores.
  - Actions: dismiss, hide message, mute (1 day / 1 week), ban from chat,
    each with a reason. Everything goes in `admin_log`.
  - Per school: turn chat on or off, list of moderators, counts (messages per
    day, open reports, oldest open report).
- Self-harm reports get a fixed note with what to do (tell a mentor or
  counsellor, link to 113 Zelfmoordpreventie / Kindertelefoon), because a
  moderator who is 16 should not be handling that alone.

**People**

- At least two moderators per school before chat goes on there, ideally one
  of them staff.
- Tell the school before turning it on at JFC.

## Web app

- New *Chat* tab in the bottom navigation, only when chat is on for the
  current account's school.
- Room list: groups (named after the subject, using the existing subject
  names) and DMs, unread counts.
- Room view: messages, input, long-press a message for *Melden* / *Verwijderen*,
  tap a name for *Blokkeren* / *Bericht sturen*.
- Lesson in the schedule gets a "Chat" link to its group's room. This is the
  feature that sells it.
- First time opening chat: a short screen with the rules (be normal, reports
  are read by moderators, what isn't allowed) that must be accepted.
- All strings in `web/src/lib/i18n.tsx`, NL and EN.
- Release note in `web/src/content/releases.ts` when it ships.
- Screenshot steps in `web/scripts/screenshots.cjs` (room list, a group chat)
  using the demo school with generated messages.

## Scale

One school (about 2,300 students) at 100 messages each per day is about
230,000 rows a day. SQLite in WAL mode with one writer is fine for that, and Go
handles thousands of open SSE connections. Things to watch when more schools
turn it on:

- **AI check costs and limits.** Batch, and if needed check only group chats
  plus reported DMs. One setting to switch it off.
- **Database size.** Retention keeps it bounded. Move to Postgres only if
  write contention shows up.
- **Moderation load.** This is the one that really doesn't scale. Hence chat
  only at schools with their own moderators.

## Legal

- **AVG**: messages are personal data of minors. Privacy notice update,
  retention as above, export and delete through the existing profile delete.
- **DSA**: as a hosting service Melon needs a way to report content (the report
  button) and to tell people why their message was removed (the reason on a
  hide or mute is shown to them).
- Let the school know before launch.

## Build order

1. Store + schema + tests (rooms, members, messages, blocks, reports).
2. DMs over the REST API, with rate limit and block checks. Polling only.
3. SSE hub.
4. Web UI for DMs.
5. Group membership from schedules, group chats in the UI, schedule link.
6. Reports + moderator dashboard + per-school switch. **Chat does not go on
   anywhere before this step is done.**
7. Word list, then the AI check.
8. Demo data, screenshots, release note, privacy notice. Launch at JFC.
