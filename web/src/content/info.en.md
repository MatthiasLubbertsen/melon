# About Melon

Melon is your Zermelo schedule, but clear. You see changes and cancellations at a glance, you can use several accounts side by side, and your schedule also works in your calendar, through an API and in AI assistants.

Melon is not part of Zermelo or of your school. It is a separate program that, with your permission, fetches your schedule from Zermelo. The code is open: [github.com/MatthiasLubbertsen/melon](https://github.com/MatthiasLubbertsen/melon).

## How it works

1. In Zermelo you create a **link code** (in the Zermelo app or on your school's portal, under *Koppelingen*).
2. Melon exchanges that code once for a **token**: a key that lets Melon read your schedule. That exchange is the only thing Melon ever sends to Zermelo; after that it only reads.
3. When you open your schedule, Melon asks Zermelo for it. An answer is kept for 30 seconds so Zermelo is not flooded. If Zermelo is down, you see the last schedule Melon had (at most 30 minutes old), clearly marked.
4. Melon makes what Zermelo sends readable: subjects get their full names, teacher codes get a name if you fill one in, and changes are explained briefly.

## Your profile

Your profile starts out as the browser you open Melon in. You can link as many Zermelo accounts to it as you like. Set a **PIN** and you can sign in on another device with your Zermelo user number and that PIN.

A PIN is short, so Melon defends against guessing: after five wrong tries signing in is locked for 15 minutes, and every further mistake doubles that, up to a day.

## Privacy

- **Your Zermelo token** is stored encrypted (AES-256-GCM) and is never sent to your browser, the API or apps. Only an admin can look at it when helping you (see below).
- **PINs, sessions, API keys and app authorizations** are stored only as hashes. You see an API key once; after that Melon itself cannot show it again.
- **Calendar links** contain a secret code. You can make a new one, and the old one stops working.
- **No tracking.** Melon uses no ads or analytics services and only sets a cookie to keep you signed in. Your browser itself remembers your language and the account you last picked.
- **Usage is only counted in totals.** Melon counts per day how many schedules were requested through the app, the API, apps and calendars, not who did it. Your IP address is only used briefly in memory to slow down abuse, and is not stored.

### What admins can see

The admins of this Melon server can, to help people, look at profiles: which accounts are linked, whether there is a PIN, and which API keys and apps exist (without the keys themselves). When something is wrong they can temporarily see Melon the way you see it, or look at and check your Zermelo token. Everything an admin does like this goes in a log.

## Deleting everything

Under **Settings → Accounts → Erase everything** you remove your profile with everything in it: linked accounts and their tokens, PIN, API keys, connected apps, calendar links and the names you gave. You can also unlink a single account; Melon then only forgets that account and its token.

If you also want Zermelo to forget the link, remove it in Zermelo under *Koppelingen*.
