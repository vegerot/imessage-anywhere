# 🔬 Experiments

Small experiments that decided parts of the plan. Each prints counts or IDs only, never Message content. Run them from a Rex terminal (it holds the Full Disk Access grant).

## E1: received images, by type and by presence on disk (2026-11-12)

Decides ticket 05. Result: 69 of 102 images received in 180 days were HEIC, and only 2 of the 102 were on the Host Mac's disk.

```sh
sqlite3 -readonly ~/Library/Messages/chat.db "
select a.uti, count(*) from attachment a
join message_attachment_join maj on maj.attachment_id = a.ROWID
join message m on m.ROWID = maj.message_id
where m.is_from_me = 0
  and m.date > (strftime('%s','now','-180 days') - 978307200) * 1000000000
  and a.uti in ('public.heic','public.png','public.jpeg')
group by 1 order by 2 desc;"
```

Presence on disk: the same query selecting `a.filename`, then `os.path.exists(os.path.expanduser(filename))` in Python for each row.

## E2: several subscriptions on one `imsg rpc` child (2026-11-12)

Decides the SSE design. Result: two `watch.subscribe` calls got independent streams with their own cursors. Script: [`e2-two-subscriptions.py`](e2-two-subscriptions.py) (replays existing rows of the note-to-self Conversation; sends nothing).

## E3: contact names (2026-11-12)

Decides ticket 06. Result: `imsg`'s Contacts permission request never prompted from a Rex terminal (status stays `notDetermined`; check with `CNContactStore.authorizationStatus(for: .contacts)`), so no names. With `SSH_CONNECTION` set, imsg reads the AddressBook database under Full Disk Access instead: 7 of 20 recent Conversations got names.

```sh
SSH_CONNECTION="local 0 local 0" imsg chats --limit 20 --json </dev/null \
  | python3 -c 'import json,sys; c=[json.loads(l) for l in sys.stdin]; print(sum(1 for x in c if x.get("contact_name")), "of", len(c))'
```
