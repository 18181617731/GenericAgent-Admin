# GA Admin /chat composer picker style fix checkpoint

Date: 2026-06-25
Repo: C:\\Users\\Fwind43\\Desktop\\code\\GenericAgent-Admin-Go

## Issue
`/chat` composer model and reasoning selectors were visually collapsed/misaligned. Runtime DOM on 8799 showed `.oa-model-select .oa-cselect > button` only ~42px wide while parent chip was 176px; selected model label span had `clientWidth` ~25 and `scrollWidth` ~265.

## Findings
- Model/reasoning selector uses `CustomSelect` (`.oa-cselect`) in `web/src/ChatApp.jsx`.
- Multiple late CSS overrides in `web/src/style.css` affect `.oa-composer-bar`, `.oa-composer-model`, `.oa-effort-select`.
- Root cause: button inside `.oa-cselect` did not occupy the chip width; model/effort wrapper sizes existed but inner button collapsed.
- Per `ga_admin_chat_sop`: after web changes run `npm --prefix web run build`, then `go build`, because Go embeds `web/dist`.
- Existing 8799 process PID 145876 runs `GenericAgent-Admin-Go.exe -port 8799 -no-browser` and still serves old embedded asset `index-CD8xx4P6.css`.

## Applied patch
Appended late CSS override in `web/src/style.css` to make composer select internals full-width and constrain model/effort flex sizes.

## Validation so far
- `npm.cmd --prefix web run build`: passed.
- Built new test exe: `go build -o temp_run_logs/ga-admin-stylefix.exe .`: passed.
- Started test process: PID 139116 on `http://127.0.0.1:8801` with `-headless -no-browser`.

## Next
Open/validate `http://127.0.0.1:8801/chat`: ensure served CSS is new `index-BXo3wgCK.css` and computed widths show model button filling parent. If good, optionally rebuild/replace/restart the real 8799 exe only with user approval or precise safe restart.
