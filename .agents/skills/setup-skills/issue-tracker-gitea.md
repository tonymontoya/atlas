# Issue tracker: Gitea

Issues and PRDs for this repo live as issues on a Gitea instance. Use the **Gitea REST API**
over `curl` for all operations (the `tea` CLI works too if installed, but `curl` is portable).

## Connection (derive from the repo)

The remote, owner, repo, host, and token are all derived from the local checkout — no hardcoding:

```sh
REMOTE=$(git remote get-url origin)
# e.g. https://git.example.com/owner/repo.git  or  git@git.example.com:owner/repo.git
ORIGIN=$(printf '%s' "$REMOTE" | sed -E 's#(https?://|.*@)([^/]+/[a-zA-Z0-9._-]+)\.git$#\2#; s#(https?://|.*@)([^/]+/[a-zA-Z0-9._-]+)$#\2#')
HOST=$(printf '%s' "$REMOTE" | sed -E 's#(https?://|.*@)([^/:]+).*#\2#')
PROTO=$(printf '%s' "$REMOTE" | sed -nE 's#^(https?://).*#\1#p'); [ -z "$PROTO" ] && PROTO=https
OWNER=${ORIGIN%/*}; REPO=${ORIGIN#*/}
API="$PROTO$HOST/api/v1"
# Token: prefer $GITEA_TOKEN; else the token stored for git push in ~/.git-credentials
TOKEN=${GITEA_TOKEN:-$(sed -E 's#.*://[^:]+:([^@]+)@.*#\1#' ~/.git-credentials)}
AUTH="Authorization: token $TOKEN"
```

Verify with: `curl -sS -H "$AUTH" "$API/version"`.

## Conventions

- **Create an issue**: `curl -sS -X POST -H "$AUTH" -H "Content-Type: application/json" "$API/repos/$OWNER/$REPO/issues" -d '{"title":"...","body":"...","labels":["ready-for-agent"]}'`. Use a JSON heredoc / `jq -n` for multi-line bodies. Capture `.number` from the response.
- **Read an issue**: `curl -sS -H "$AUTH" "$API/repos/$OWNER/$REPO/issues/<n>` for the body+labels; comments via `"$API/repos/$OWNER/$REPO/issues/<n>/comments"`.
- **List issues**: `curl -sS -H "$AUTH" "$API/repos/$OWNER/$REPO/issues?state=open&type=issues&labels=<comma-sep,label,names>&limit=50&page=1"`. Gitea paginates (`?page=`); loop until an empty page.
- **Comment**: `curl -sS -X POST -H "$AUTH" -H "Content-Type: application/json" "$API/repos/$OWNER/$REPO/issues/<n>/comments" -d '{"body":"..."}'`.
- **Apply labels**: `curl -sS -X POST -H "$AUTH" -H "Content-Type: application/json" "$API/repos/$OWNER/$REPO/issues/<n>/labels" -d '{"labels":["ready-for-agent"]}'`.
- **Remove a label**: `curl -sS -X DELETE -H "$AUTH" "$API/repos/$OWNER/$REPO/issues/<n>/labels/<label-name>"`.
- **Assign (claim)**: `curl -sS -X PATCH -H "$AUTH" -H "Content-Type: application/json" "$API/repos/$OWNER/$REPO/issues/<n>" -d '{"assignees":["<user>"]}'`.
- **Close**: `curl -sS -X PATCH -H "$AUTH" -H "Content-Type: application/json" "$API/repos/$OWNER/$REPO/issues/<n>" -d '{"state":"closed"}'`.

Labels must exist before they're applied. Create a label with `POST "$API/repos/$OWNER/$REPO/labels"` body `{"name","color","description"}` (Gitea assigns the numeric id; delete with `DELETE .../labels/<id>`).

## Pull requests as a triage surface

**PRs as a request surface: no.** _(Set to `yes` if this repo treats external PRs as feature requests; `/triage` reads this flag.)_

Gitea shares one number space across issues and PRs. A bare `#42` may be either — disambiguate via `GET "$API/repos/$OWNER/$REPO/issues/42"` and check `pull_request` in the response.

## Blocking edges (dependencies)

**Primary: text fallback (always works).** Put a line at the top of the child issue body:

```
Blocked by: #<n>, #<n>
```

A ticket is unblocked when every issue it lists is **closed**. This is the representation used by `to-tickets` and `wayfinder` on this repo.

**Native Gitea issue dependencies are optional** (`POST "$API/repos/$OWNER/$REPO/issues/<child>/blocks"` body `{"issue_id": <blocker>}`). They are **not** relied on here because the endpoint is disabled or unreliable on some instances — verify with a throwaway issue before depending on it. If native deps are confirmed working on this instance, you may add them in addition to the text line for UI visibility, but the text line remains the source of truth.

## When a skill says "publish to the issue tracker"

Create a Gitea issue (`POST .../issues`), with the `ready-for-agent` label unless told otherwise.

## When a skill says "fetch the relevant ticket"

`GET "$API/repos/$OWNER/$REPO/issues/<n>"` plus `/issues/<n>/comments` for the conversation.

## Wayfinding operations

Used by `/wayfinder`. The **map** is a single issue with **child** issues as tickets.

- **Map**: a single issue labelled `wayfinder:map`, holding the Notes / Decisions-so-far / Fog body. Create the label first if it doesn't exist.
- **Child ticket**: an issue whose body starts with `Part of #<map>` and carries a `Type:` line (`research`/`prototype`/`grilling`/`task`) plus `Status:` (`open`/`claimed`/`resolved`). Add the child to a task list in the map body (Gitea renders `- [ ] #<n>` as a linked task). Label: `wayfinder:<type>`.
- **Blocking**: the `Blocked by: #<n>` line (see above). Frontier eligibility requires every listed blocker closed.
- **Frontier query**: list the map's open children (label `wayfinder:*`, `state=open`), drop any with an open issue in its `Blocked by` line or with an assignee; first in map-body order wins.
- **Claim**: `PATCH .../issues/<n>` `{"assignees":["<me>"]}` and set `Status: claimed` in the body — the session's first write.
- **Resolve**: comment the answer (`POST .../comments`), then `PATCH` `{"state":"closed"}`, then append a context pointer to the map's Decisions-so-far.
