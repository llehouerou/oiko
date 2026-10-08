# Back up

Oiko produces no backup: the host owns its schedule, destination and retention. The paths below
are NixOS's `/var/lib/oiko`; elsewhere, the `-data` directory ([Install](install.md#flags)).

1. Copy `history.db` with `sqlite3 /var/lib/oiko/history.db "VACUUM INTO '/backup/history.db'"`
   (not `.backup`), and leave the live `history.db*` files (db, `-wal`, `-shm`) out.
2. Copy the JSON files of `/var/lib/oiko` and its subdirectories as they are, among them the
   Persons, Kiosks, Programs and Sessions: `persons.json`, `kiosks.json`, `programs.json` and
   `sessions.json`.
3. To restore: stop `oiko`, put the copy back as `history.db` with no `-wal`/`-shm`, and the JSON
   files as they were, then start it. The restored `alive` mark makes Oiko record a Gap from the
   backup time (up to a minute early) to the restart. Older JSON files bring back the Sessions and
   Tokens they hold, and the Persons, Kiosks and Programs: sign out, remove or revoke again what
   should stay gone.
4. A type of Bridge may keep state that must not move to another host, such as the Arlo
   session of `github.com/llehouerou/oiko-arlo`: its documentation says so.

## Audit log

The Audit log lives in `history.db`: dropping that file drops the trail. The Names of removed
Persons, Kiosks and Programs stay in it for up to a year.
