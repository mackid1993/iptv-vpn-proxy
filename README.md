# iptv-vpn-proxy

Put all of your IPTV behind one VPN, on every device, without installing a VPN
on any of them.

> [!NOTE]
> **Changelog** · Last updated: November 14, 2026

<details>
<summary>Changelog</summary>

### November 14, 2026

- **There are now three containers instead of two.** A small helper, called
  the relay, has joined the VPN and nginx. It is a short Python program that
  runs in the official Python image with nothing installed on top, and its
  whole text sits inside `compose.yaml`, the same way the nginx settings do.
  You still fill in one file, `.env`, and nothing else.
- **Each provider needs one more line in `.env`:** `NAME_LIVE_TV=hls` or
  `NAME_LIVE_TV=ts`. It says which live format your players use for that
  provider, so set it to match what is chosen in the player. There is no
  default. A provider without this line is skipped, and the status page tells
  you which line to add.
  **Why it is needed:** it fixes a real fault. Many providers allow one
  session per channel on an account, and an MPEG-TS request and an HLS
  request for the same channel count as two. So when a recorder asked for
  MPEG-TS while a player was watching that channel on HLS, each new request
  ended the other one: the picture froze and recordings broke. The proxy can
  keep everyone on a channel on a single provider session, but only if it
  knows which format your players use, and it cannot work that out by itself.
- **New status page.** Open
  `http://<the machine's address>:<any provider's port>/status` in a web
  browser. It shows what is set up, what is wrong and how to fix it, where the
  VPN comes out, the state of each provider account, and what is playing. To
  show this it asks ipinfo.io (through the VPN) where the VPN comes out, and
  asks each provider for its account details, only while the page is open.
- **MPEG-TS: one connection per channel.** Several devices watching or
  recording the same channel now share one connection to the provider. A
  device that joins a channel that is already playing starts about 10 seconds
  behind live, so it has some video in hand if the provider pauses. When you
  change channel, the old connection is let go before the new one is opened.
- **HLS: watch and record a channel at once.** A recorder that asks for
  MPEG-TS is now fed from the channel's HLS session, the one your players are
  already on. A recorder and a viewer on one channel no longer knock each
  other off.
- **HLS: the provider is let go sooner when you stop.** An idle connection to
  a provider's video server is now closed after 20 seconds instead of 55.
  Connections are still reused while a channel plays.
- **nginx is still plain nginx.** It is the stock image with settings only and
  no add-on scripting. Everything new that needed a program lives in the
  relay. If the relay is not running, TV still plays, without the sharing and
  without the status page.
- **Safety.** The proxy is stricter about refusing addresses that point at the
  machine itself or at your home network: it catches another way of writing
  such an address, and the relay checks where a provider's redirect really leads
  before it connects. The notes nginx and the relay pass to each other are
  never accepted from a player or from a provider's server, so a server
  cannot use them to steer your player outside the VPN.
- **If you are updating:** replace `compose.yaml`, add the `NAME_LIVE_TV` line
  to every provider in `.env`, then run
  `docker compose up -d --force-recreate`. Until a provider has that line it
  is skipped, and with none left nothing plays; the status page says which
  line is missing. A provider can no longer use port 8197, which the relay
  now uses inside the stack. A change to `.env` now restarts the relay along
  with nginx, which ends any shared MPEG-TS stream for a moment.

</details>

> [!TIP]
> **There is a status page.** Open
> `http://<the machine's address>:<any provider's port>/status` in a web
> browser, for example `http://192.168.1.50:8095/status`. It shows what is set
> up, what is wrong and the exact line to fix, where the VPN comes out, the
> state of each provider account, and what is playing. Look there first when
> something does not work.

Three containers run on a machine at home. Your players connect to that
machine instead of to your IPTV provider, and everything they fetch leaves
through the VPN. On the phone, TV or streaming box you type an address and a
login into your player, the same way you would with the provider's own
details. That is the whole setup on the device.

- **gluetun** holds the VPN tunnel.
- **nginx** sits inside that tunnel and passes your players' requests on to the provider.
- **the relay**, a small Python helper, sits inside the tunnel too. When
  several devices watch or record the same live channel, it lets them share
  one connection to the provider. It also draws the status page.

> **For your home network only.** The proxy ports have no protection of their
> own. Never forward them from the internet. To watch away from home, reach
> your home network privately (Tailscale, your own WireGuard, and so on).

Use it only with services you are entitled to use.

## Contents

- [Design goals](#design-goals)
- [Why this exists](#why-this-exists)
- [What you need](#what-you-need)
- [Set it up](#set-it-up)
- [The two files](#the-two-files)
- [Everyday use](#everyday-use)
- [With a playlist manager](#with-a-playlist-manager)
- [How it works](#how-it-works)
- [What was tested, and how](#what-was-tested-and-how)
- [Trade-offs and things to know](#trade-offs-and-things-to-know)
- [What it stores, updating, removing](#what-it-stores-updating-removing)
- [Extras that need one more file](#extras-that-need-one-more-file)
- [Platform notes](#platform-notes)
- [When something is wrong](#when-something-is-wrong)
- [How it works (technical)](#how-it-works-technical)
- [Support](#support)
- [License](#license)

## Design goals

1. **Parity with direct credentials.** A player pointed at the proxy should
   behave exactly as if you had typed the provider's own Xtream login into it.
   Not better, not different: the same, with the VPN added.
2. **One VPN for everything.** Every device and every player goes out through
   the same tunnel, and no device needs a VPN app.
3. **The easiest setup that can work.** One settings file. You never edit the
   compose file. Adding a provider is five lines.
4. **Works with any player and any Xtream provider.** Nothing in it is written
   for one provider or one app.
5. **Nothing to build or install.** It is stock gluetun, stock nginx and the
   stock Python image, with nothing added to any of them. Everything that
   makes them a proxy is text inside the compose file, where you can read it:
   the nginx settings, a few lines of shell that read your settings at start
   and keep a list of the video servers your streams come from, and one
   Python program (the relay) that uses only what Python ships with.

## Why this exists

A plain reverse proxy in front of an IPTV provider usually works for MPEG-TS
and then freezes every few seconds on HLS. The cause:

- Many providers start a **new session** every time a channel's address is asked for.
- A new session **ends the previous one** on that channel.
- An HLS player asks for that address again every few seconds. A second device
  or a recording on the same channel asks too.

A proxy that makes a fresh trip to the provider for each of those requests
keeps ending its own session, so the pieces the player was about to download
stop existing. A player connected directly does not have this problem, because
it follows the provider's redirect once and then stays on that session.

This proxy does what the direct player does. It remembers where the provider
sent each channel and keeps every reader of that channel on that one session.

MPEG-TS has the same problem as soon as a second device or a recorder opens a
channel that is already playing. With a provider that allows one connection
per channel, the readers end each other over and over. So the relay opens each
live MPEG-TS channel at the provider once and gives every reader of that
channel the same video.

## What you need

- A machine at home that is always on, with Docker and Docker Compose 2.24 or newer.
- A VPN that gluetun supports, or a WireGuard or OpenVPN config file from any VPN.
- IPTV providers that give you an **Xtream login**: a server address, a username and a password.

## Set it up

1. Make a folder and put `compose.yaml` and `.env.example` in it. Both are in
   this repository.
2. Copy `.env.example` to `.env`.
3. Fill in `.env`. It has four sections and explains each one:
   your VPN, the easy login you choose, the port range, and your providers.
   Each provider is five lines. The fifth, `NAME_LIVE_TV`, is `hls` or `ts`:
   whichever live format is chosen in your player for that provider.
4. Start it:
   ```
   docker compose up -d
   ```
5. Check that it works. Open the status page in a web browser:
   ```
   http://<the machine's address>:<any provider's port>/status
   ```
   It says whether the proxy is working, lists each provider, and names
   anything that is wrong along with the line to fix. The log of the
   `iptv-nginx` container says the same in short at each start:
   ```
   PROVIDER PROVIDER1 on port 8095 -> https://provider.example
   STATUS PAGE: open http://<this machine's address>:8095/status in a web browser
   ```
   A provider with a problem is listed as `SKIPPED` with the reason, and the
   others still start. Each of the three containers keeps a log:
   ```
   docker logs iptv-nginx
   docker logs iptv-relay
   docker logs iptv-vpn
   ```
6. In your player or playlist manager, add an **Xtream** source:
   - server: `http://<the machine's address>:<that provider's port>`
   - username and password: the `PLAYER_USER` and `PLAYER_PASSWORD` you chose

Each provider gets its own port. The same easy login works for all of them.

## The two files

`.env.example` is shown here in full so you can read it without leaving the
page. Click the heading to open it. `compose.yaml` is linked instead, because
it is about 2,000 lines long.

### `.env.example`: every setting, with instructions

Copy it to `.env` and fill it in. This is the only file you edit. It covers the
VPN, the login your players use, the ports, and one block of five lines per
provider.

<details>
<summary><b>Show .env.example</b></summary>

```sh
##############################################################################
# Settings for the IPTV VPN proxy. Copy this file to ".env" and fill it in.
#
# This one file holds everything:
#   1. your VPN (the tunnel everything goes out through)
#   2. the one easy login your apps use with the proxy
#   3. the ports the proxy may use
#   4. one block of five lines per IPTV provider
#
# You never edit compose.yaml. Change this file, then bring the stack up
# again ("docker compose up -d", or the Up / Deploy / Build button of your
# Docker manager). Compose notices the change and restarts what needs it:
#   - a change to a provider or to the easy login restarts only the proxy
#     and its helper (the relay); the VPN stays up
#   - a change to a VPN setting or to the port range also restarts the tunnel,
#     which interrupts whatever is playing for a few seconds
#
# Lines starting with # are notes and are ignored.
# Write values plainly after the = sign, with no spaces around the =.
# A value containing anything besides letters, digits and  . - _ : / ,
# goes between single quotes:   SOMETHING='p@ss w#rd$1'
##############################################################################

# Your time zone, for the times in the logs (e.g. America/New_York, Europe/London).
TZ=Etc/UTC


##############################################################################
# 1. THE VPN
#
# The tunnel is made by gluetun, and every line in this section is one of
# gluetun's own settings, described in its documentation:
#     https://github.com/qdm12/gluetun-wiki   (look up your VPN provider
#     under "setup/providers" for the exact lines it needs)
# The settings shown in this section are most of the ones this stack passes on
# to gluetun (SERVER_REGIONS and SERVER_HOSTNAMES are passed on too). The
# README says how to add a different one.
#
# Pick ONE of the four ways below, remove the # from its lines and fill them
# in. Leave the others commented out.
##############################################################################

# ---- Way 1: WireGuard, a VPN provider gluetun knows by name ---------------
# The name is your VPN company in lower case, for example mullvad, protonvpn,
# nordvpn, surfshark, airvpn or ivpn. WIREGUARD_ADDRESSES is the
# "Address" line of the WireGuard file your provider gives you. The preshared
# key is only for providers that give one. The last two lines are optional
# and choose where you exit.
# VPN_SERVICE_PROVIDER=mullvad
# VPN_TYPE=wireguard
# WIREGUARD_PRIVATE_KEY=
# WIREGUARD_ADDRESSES=10.64.222.21/32
# WIREGUARD_PRESHARED_KEY=
# SERVER_COUNTRIES=Netherlands
# SERVER_CITIES=Amsterdam

# ---- Way 2: WireGuard, from a .conf file of any provider or your own server
# Copy each value from the .conf file:
#   WIREGUARD_PRIVATE_KEY     "PrivateKey" under [Interface]
#   WIREGUARD_ADDRESSES       "Address" under [Interface]
#   WIREGUARD_PUBLIC_KEY      "PublicKey" under [Peer]
#   WIREGUARD_PRESHARED_KEY   "PresharedKey" under [Peer], if there is one
#   WIREGUARD_ENDPOINT_IP     "Endpoint", the part before the colon
#   WIREGUARD_ENDPOINT_PORT   "Endpoint", the part after the colon
# VPN_SERVICE_PROVIDER=custom
# VPN_TYPE=wireguard
# WIREGUARD_PRIVATE_KEY=
# WIREGUARD_ADDRESSES=10.0.0.2/32
# WIREGUARD_PUBLIC_KEY=
# WIREGUARD_PRESHARED_KEY=
# WIREGUARD_ENDPOINT_IP=203.0.113.10
# WIREGUARD_ENDPOINT_PORT=51820

# ---- Way 3: OpenVPN, a VPN provider gluetun knows by name ------------------
# For example nordvpn, expressvpn, cyberghost or surfshark. The protocol is
# udp (faster) or tcp (gets through more networks). SERVER_COUNTRIES is
# optional.
# VPN_SERVICE_PROVIDER=nordvpn
# VPN_TYPE=openvpn
# OPENVPN_USER=
# OPENVPN_PASSWORD=
# OPENVPN_PROTOCOL=udp
# SERVER_COUNTRIES=Netherlands

# ---- Way 4: OpenVPN, from an .ovpn file ------------------------------------
# gluetun has to be handed the file itself. That takes one small extra file
# next to compose.yaml; the README shows it ("Extras that need one more
# file"). The login lines are only needed if the file asks for one.
# VPN_SERVICE_PROVIDER=custom
# VPN_TYPE=openvpn
# OPENVPN_CUSTOM_CONFIG=/gluetun/custom.ovpn
# OPENVPN_USER=
# OPENVPN_PASSWORD=

# ---- Sometimes needed with WireGuard ---------------------------------------
# Lower the MTU if streams stall or pages half-load (try 1320, then 1280).
# WIREGUARD_MTU=1320
# Keep the tunnel awake if it drops when idle.
# WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL=25s
# Use this if the machine's kernel has no WireGuard (some NAS models, WSL 2).
# WIREGUARD_IMPLEMENTATION=userspace

# ---- Optional, for any of the four ways -----------------------------------
# Your players reach the proxy without any setting here.
#
# FIREWALL_OUTBOUND_SUBNETS is for the other direction: networks the
# containers may connect OUT to directly, outside the tunnel. Leave it out
# unless you know you need it.
# FIREWALL_OUTBOUND_SUBNETS=192.168.1.0/24
#
# Which DNS the tunnel uses. By default gluetun uses encrypted DNS and blocks
# known-malicious names; leave these out unless you know you need them.
#
# To use one particular DNS server instead (your VPN provider's own, for
# example), set the two lines below. THE ADDRESS MUST END IN :53, the DNS
# port. Without the port gluetun does not accept the address and nothing can
# look up names. For several servers, separate them with commas.
#     DNS_UPSTREAM_RESOLVER_TYPE=plain
#     DNS_UPSTREAM_PLAIN_ADDRESSES=10.0.0.1:53
# (for example: DNS_UPSTREAM_RESOLVERS=quad9   or   BLOCK_MALICIOUS=off)


##############################################################################
# 2. THE LOGIN THE PROXY ANSWERS TO
#
# This is the easy username and password you type into your player or
# playlist manager, for every provider. The proxy swaps it for that
# provider's real login on the way out, so the real logins are typed in one
# place only: this file. The same easy login works for every provider; the
# PORT picks the provider.
# You choose these two. Use letters, digits, - and _ only.
##############################################################################
PLAYER_USER=choose-a-name
PLAYER_PASSWORD=choose-a-password


##############################################################################
# 3. THE PORTS THE PROXY MAY USE, as a range: FIRST-LAST
#
# Every provider below needs its own port inside this range. Set it to
# whatever ports are free on your machine: 8095-8096 is room for two
# providers, 8095-8099 for five. Widen or narrow it any time and bring the
# stack up again (that one change also restarts the VPN).
##############################################################################
PROXY_PORTS=8095-8096


##############################################################################
# 4. YOUR PROVIDERS
#
# Each provider is a block of five lines that share one NAME:
#
#     NAME_URL=https://provider.example      the server address the provider
#                                            gave you: http:// or https://,
#                                            the server name, optionally
#                                            :port, and nothing after that
#     NAME_PORT=8095                         a free port inside PROXY_PORTS
#     NAME_USER=the-username-they-gave-you
#     NAME_PASSWORD=the-password-they-gave-you
#     NAME_LIVE_TV=hls                       how your players watch its live
#                                            channels: hls or ts (see below)
#
# NAME is any word you like in CAPITALS (letters, digits, _), for example
# PROVIDER1 or SPORTS. It is only a label; all five lines must use the same
# one.
#
# NAME_LIVE_TV: HLS OR MPEG-TS. Say which live format your players use for
# this provider. There is no default, and a provider without this line is
# left out.
#     hls   Your players use HLS. Everyone watching a channel shares one
#           connection to the provider. A recorder that asks for MPEG-TS is
#           given the same video as your players, so you can watch and
#           record a channel at once.
#     ts    Your players use MPEG-TS. Everyone on a channel shares one
#           connection to the provider and gets the provider's own stream,
#           so you can watch and record a channel at once. Do not open a
#           channel as HLS while it is playing or recording as MPEG-TS:
#           most providers allow one kind at a time per channel.
# Use the format chosen in your players, and in your playlist manager if you
# have one. They all have to agree. If a provider offers only one of the
# two, use that one.
#
# PROVIDER LOGINS CAN CONTAIN ANY CHARACTERS. You do not get to choose them,
# so the proxy takes whatever the seller gave you. One rule for THIS FILE: if
# a username or password has anything in it besides letters and digits, put
# it between single quotes, exactly as given:
#        NAME_PASSWORD='p@ss w#rd$1%'
# Without the quotes, a $ or a # in the value is read as something else and
# part of the login is silently lost. If the login itself contains a single
# quote, write that one character as \'  (backslash, then the quote).
#
# TO ADD A PROVIDER
#   1. Copy the example block below, remove the # in front of its five
#      lines, change NAME, and fill in the values.
#   2. Bring the stack up again.
#   3. Check it was picked up: the iptv-nginx container's log lists the
#      providers found at every start, like
#          PROVIDER PROVIDER1 on port 8095 -> https://provider.example
#      A provider with a problem is listed as SKIPPED with the reason, and
#      the other providers keep working.
#      The status page shows the same and more. Open it in a web browser:
#          http://<this machine's address>:<any provider's port>/status
#   4. In your player or playlist manager add an Xtream source with
#          server    http://<this machine's address>:<NAME_PORT>
#          username  the PLAYER_USER above
#          password  the PLAYER_PASSWORD above
#
# TO REMOVE A PROVIDER: delete its five lines (or put # in front of them)
# and bring the stack up again.
#
# A provider's port is what tells the proxy which provider you mean, so if
# you change a port, change it in your apps too.
##############################################################################

# ---------- example (remove the # signs to use it) ----------
# PROVIDER1_URL=https://provider.example
# PROVIDER1_PORT=8095
# PROVIDER1_USER=the-username-they-gave-you
# PROVIDER1_PASSWORD=the-password-they-gave-you
# PROVIDER1_LIVE_TV=hls

# ---------- a second one ----------
# PROVIDER2_URL=http://another-provider.example:8080
# PROVIDER2_PORT=8096
# PROVIDER2_USER=the-username-they-gave-you
# PROVIDER2_PASSWORD='the-p@ssword-they-gave-you'
# PROVIDER2_LIVE_TV=hls

```

</details>

### `compose.yaml`: the stack itself

You do not edit this one. Get it from this repository:
[compose.yaml](compose.yaml). It defines the three containers and holds, as
plain text with a comment on each part saying what it is for:

- the whole nginx configuration
- the short shell script that reads your settings when nginx starts
- the relay, a Python program of about 1,000 lines that uses only what Python
  ships with

Nothing is downloaded or installed when the stack starts except the three
stock images.

## Everyday use

**Add a provider.** Add its five lines to `.env`, using a name of your choice
in capitals and a free port inside `PROXY_PORTS`:

```
SPORTS_URL=https://provider.example
SPORTS_PORT=8096
SPORTS_USER=the-username-they-gave-you
SPORTS_PASSWORD='the-p@ssword-they-gave-you'
SPORTS_LIVE_TV=hls
```

Then run `docker compose up -d`. Only the proxy and the relay restart; the VPN
stays up.

**Pick the live format.** `NAME_LIVE_TV` says how your players watch that
provider's live channels. There is no default, and a provider without the
line is left out.

- `hls`: your players use HLS. Everyone watching a channel shares one
  connection to the provider. A recorder that asks for MPEG-TS is given the
  same video as your players, so you can watch and record a channel at once.
- `ts`: your players use MPEG-TS. Everyone on a channel shares one connection
  to the provider and gets the provider's own stream, so you can watch and
  record a channel at once. Do not open a channel as HLS while it is playing
  or recording as MPEG-TS: most providers allow one kind at a time per
  channel.

Use the format chosen in your players, and in your playlist manager if you
have one. They all have to agree. If a provider offers only one of the two,
use that one. If it offers both and you have no reason to prefer one, use
`hls` (see [Trade-offs and things to know](#trade-offs-and-things-to-know)).

**Remove a provider.** Delete its five lines and run `docker compose up -d`.

**Logins with odd characters.** Provider logins can contain anything. You do
not get to choose them, so the proxy takes whatever you were given. One rule
for the `.env` file: if a username or password has anything besides letters
and digits, put it between single quotes. Without them, a `$` or `#` is read
as something else and part of the login is silently lost.

**More providers than ports.** Widen `PROXY_PORTS` (for example `8095-8099`)
and run `docker compose up -d`. This one restarts the VPN too.

**See what is happening.** The status page
(`http://<the machine's address>:<any provider's port>/status`) shows what is
set up, where the VPN comes out, each provider's account, and how many live
channels are playing. For more detail, each container has a log:

- `docker logs iptv-nginx` has the providers found at the last start, then one
  line per request.
- `docker logs iptv-relay` has a line each time a live MPEG-TS channel
  (shared, or fed from HLS) is opened or closed, how long the provider took to send the first video, and anything
  that went wrong with a channel.
- `docker logs iptv-vpn` is the VPN tunnel's own log.

Addresses are never logged by nginx or the relay, because they contain logins.

## With a playlist manager

A playlist manager is the tool some people use to trim, rename and reorder
what a provider offers. The proxy works with one, and where the proxy goes
depends on where the manager runs.

**A manager you run yourself, on your own network**

```
player  ->  playlist manager  ->  proxy  ->  provider
```

- In the manager, each provider source uses the proxy's address and port,
  with your easy login.
- Your players point at the manager, as they would without the proxy.
- The provider's real login lives only in the proxy's `.env`.
- The live format has to agree everywhere: the one chosen in your players,
  the one the manager hands out, and the provider's `NAME_LIVE_TV` line.

**A hosted manager, running on someone else's servers**

```
player  ->  proxy  ->  playlist manager  ->  provider
```

A hosted manager cannot reach a proxy inside your house, so the proxy goes in
front of it and treats the manager as the provider.

- In `.env`, add the manager as a provider: its server address as the URL,
  and your login for the manager as the username and password.
- Your players point at the proxy, with your easy login.
- When something is played, the manager answers "go to the provider's
  server". The proxy rewrites that answer as it does any redirect, so the
  video goes out through the VPN.
- The hosted manager itself holds your provider's real login and fetches the
  provider's lists from its own servers. That part does not go through your
  VPN; only what your players fetch does.

The first arrangement is the one this was built and tested with. The second
is the same mechanism but has not been run on this version. In particular,
live HLS through a hosted manager adds one more redirect on the way to the
video, and whether a channel's session is still shared across it has not been
tested.

In either arrangement, see the note on pictures under
[Trade-offs and things to know](#trade-offs-and-things-to-know).

## How it works

Most of what the proxy does is pass bytes along. Eight things go beyond that.
The first seven exist to keep the behavior the same as a direct connection,
and the last is there to help you. Engineers will find the details under
[How it works (technical)](#how-it-works-technical).

1. **Redirects stay inside the tunnel.** Providers answer most requests with
   "go to that other server". The proxy rewrites that answer so the player
   comes back to the proxy, which then fetches from the other server through
   the VPN. Without this the player would go to that server directly.
2. **Playlists and picture addresses are pointed back at the proxy.** An HLS
   playlist is a list of video pieces, and a provider's channel lists name a
   logo or poster for each entry. The proxy rewrites those addresses for the
   same reason.
3. **One session per live HLS channel.** Described [above](#why-this-exists).
   When the provider ends a session, the proxy opens a new one once and
   carries on.
4. **One easy login.** You choose a simple username and password for your
   players. The proxy swaps it for the provider's real login on the way out,
   so the real logins are typed in one place only.
5. **It refuses to be pointed at your own network.** A request cannot make the
   proxy fetch from the machine it runs on or from private addresses.
6. **Connections are kept open and reused.** A player connected directly keeps
   its connection to a server open and sends one request after another over
   it. The proxy does the same on both sides. A playlist answer leaves the
   player's connection open for the next refresh, and the proxy keeps its own
   connections to the provider's video servers open instead of setting one up
   for every playlist refresh and every video piece. Setting up an encrypted
   connection takes time, and one slow setup can use up a player's small live
   buffer and show as a stutter.
7. **One provider connection per live channel for MPEG-TS too.** This is the
   relay's main job. With `NAME_LIVE_TV=ts`, the first reader of a channel
   makes the relay open it at the provider, and everyone else on that channel
   is given the same video. With `NAME_LIVE_TV=hls`, a reader that asks for
   MPEG-TS (many recorders do) is given the channel's HLS video joined end to
   end, so it shares the session your players are on.
8. **A status page.** The relay draws a page that shows what is set up, what
   is wrong, where the VPN comes out and what is playing.

Your settings are read once, when the proxy starts. A short shell loop turns
each provider block in `.env` into its own listener on its own port.

No video server is named anywhere in the two files, because a provider can
move to other servers at any time. The proxy learns them instead:

- It notes the name of each server that video or a playlist really came from.
- A second short loop gives each name it has not seen a place on its list of
  servers to keep connections open to. It waits about 20 seconds for channel
  changes to settle, then has nginx re-read its settings once for all the
  names it collected.
- Until then, a server it has not seen works the plain way, with a connection
  set up for each request.
- The list is kept in a small Docker volume and loaded at every start, so a
  server is learned once. It is capped at 200 names, far more than a provider
  uses.
- Only video servers reached by name over HTTPS on the standard port are
  learned. Any other server, and one that only hands out pictures, works the
  plain way.

nginx runs as a single worker, so every stream shares the same set of open
connections.

If the relay is not running, TV still plays. nginx passes live MPEG-TS to the
provider itself, each reader on a connection of its own, and rewrites
playlists itself. Only the sharing in point 7 and the status page are missing
until the relay is back.

## What was tested, and how

The goal is parity with direct credentials, and no gap has been seen so far
on the real setup it was run on. Some of that was confirmed against real providers and some only
against simulated ones, so here is which is which.

**The relay is newer than the rest.** Its parts (MPEG-TS sharing, the
HLS-to-MPEG-TS bridge, playlist rewriting by the relay, the status page) were
run on one real setup on November 14, 2026, and against simulated providers.
On the real setup: three
players shared one MPEG-TS channel on one provider connection, joining and
leaving without disturbing each other; HLS played with playlists rewritten by
the relay; and an MPEG-TS reader was fed from a channel's HLS session while an
HLS player watched the same channel. That is hours of use, not weeks. The
lists below describe the proxy as it was before the relay was added.

**Confirmed against real providers:**

- Live TV over HLS and over MPEG-TS
- Movies, including skipping around in them
- Catch-up and time shift
- The guide and the provider's lists
- Changing channels
- Several readers on the same channel at once, over HLS
- Scheduled recordings
- A session ended by the provider being replaced without a failed request
- Restarting the proxy while a channel is playing
- Logos and posters loading through the proxy
- Several opens of one channel arriving at the same moment ending up on one
  session
- A playlist refresh reusing the player's open connection
- Video servers being learned from what was played, and connections to them
  being reused for playlists and video pieces
- The learned servers still being known after the proxy was restarted

Connection reuse was measured on a real provider by reading the proxy's log
while live channels played. About 99 percent of requests reused an open
connection, and the rest connected in a few hundredths of a second, with no
visible stutter over a long viewing session. Without reuse, every playlist
refresh and every video piece set up its own encrypted connection, and an
occasional one took about a second, which was enough to empty the player's
live buffer and show as a stutter.

**Confirmed against simulated providers only.** These are behaviors a provider
could have that the real ones used for testing did not, so a stand-in was
built for each. All of them pass, but none has been seen against a real
service:

- Playlists that name their pieces by bare file name (`chunk_123.ts`),
  including a further playlist in a sub-folder
- A provider that answers with the playlist itself instead of a redirect
- Redirects written in less common forms
- Playlist lines written as `//server/path` (another server, same http or
  https)
- Playlist lines written as `../folder/file` in a live channel's playlist,
  with a player that works the `..` out itself and with one that sends it as
  written. A few rarer forms are not handled there: `./../x`,
  `folder/../../x`, a bare `..`, and a line that climbs more than 32 folders
  above the server's top folder.
- Playlists labeled with an unusual content type
- Logins full of special characters (`@ # % & + = ? / " $ ; { } \`, an
  apostrophe, non-English letters)

**Not run:** the [platform notes](#platform-notes) other than plain Linux
with Docker Compose. They are written from each platform's documentation.

## Trade-offs and things to know

- **Xtream logins only.** A provider's ready-made M3U link (`get.php`) is
  refused, because the addresses inside it would skip the VPN. If an M3U link
  is all you were given, the Xtream login is inside it:
  `http://SERVER/get.php?username=USERNAME&password=PASSWORD&...`
- **Use the address you typed.** When a player logs in, the provider's reply
  includes the provider's own server address. The proxy passes that reply on
  unchanged. A player that connected to that address, instead of the proxy
  address you typed in, would reach the provider directly and skip the VPN.
- **Logos and posters go through the VPN too, with one exception.** A
  provider's channel and movie lists give each picture as a full web address,
  usually on some other site. The proxy points those addresses back at
  itself, so the player fetches the pictures through the tunnel. The
  exception is pictures named inside the guide file (`xmltv.php`): that file
  is passed on as is, so a player that shows guide artwork fetches it
  directly. They are only pictures; no video comes that way.
- **If a playlist manager sits between your player and the proxy,** the
  pictures are its business, not the proxy's. A playlist manager hands the
  player its own list, often with logos from its own picture library and with
  addresses it saved earlier. The proxy never sees those, so they are fetched
  directly. Only a player that talks to the proxy itself gets every picture
  through the VPN.
- **Pick one live format per provider and use it everywhere.** With
  `NAME_LIVE_TV=ts`, do not open a channel as HLS while it is playing or
  recording as MPEG-TS: most providers allow one kind at a time per channel,
  and the two will cut each other off.
- **If a provider offers both formats, HLS is the recommended setting.**
  MPEG-TS is one continuous stream, so how smoothly it plays depends on how
  evenly the provider sends video. The relay passes it on as it arrives and
  cannot smooth out a provider that sends in fits and starts.
- **A device that joins a channel is a little behind live.** With `ts`, a
  device that joins a channel someone else is already on starts about 10
  seconds behind. With `hls`, a recorder that asks for MPEG-TS gets the HLS
  video, which is a few seconds further behind live than the provider's own
  MPEG-TS.
- **The status page asks two outside parties questions.** To show where the
  VPN comes out it asks `ipinfo.io`, through the VPN. To show each account's
  state it asks the provider, through the proxy, at most once every 15
  seconds and only while someone has the page open. The page shows no logins, but like
  the proxy ports it is open to anyone on your home network.
- **If the relay stops, TV still plays.** nginx then passes live MPEG-TS to
  the provider itself, each reader on a connection of its own, and rewrites
  playlists itself. Sharing and the status page come back with the relay.
- **Your logins are in `.env` in plain text**, and the session memory
  (below) holds them too. Keep the folder private.
- **A video server is learned the first time it is used.** Until the proxy
  has seen a video server, each request to it sets up its own connection, as
  a plain proxy would. About 20 seconds after the channel changes settle, the
  proxy re-reads its settings once so it can keep connections to that server
  open, and that makes the player reconnect once. After that the server is
  remembered, across restarts too.
- **Kept-open connections cover video servers reached by name over HTTPS on
  the standard port.** That is how most providers serve video. A provider
  that serves it over plain HTTP, on another port, or from a bare number
  address still plays: each request to such a server sets up its own
  connection, as a plain proxy would.
- **A slow provider is still slow.** If a provider's server falls behind, the
  proxy cannot make up the difference. See
  [When something is wrong](#when-something-is-wrong) for how to tell that
  apart from a proxy problem.
- **Changing a VPN setting interrupts playback** for a few seconds while the
  tunnel reconnects. Changing a provider does not touch the VPN.

## What it stores, updating, removing

**What it stores.** Besides the two files you put there, two small Docker
volumes. Docker creates and manages them; there is no folder for you to make
or look after.

`<folder>_hls-memory` (for example `iptv-vpn_hls-memory` when the folder is
named `iptv-vpn`) is the session memory:

- For each live HLS channel being watched it holds one file of about 3 KB: the
  provider's "this channel is on that server" answer. The file includes the
  channel's address at the provider, **with your real login in it**.
- A channel's file is dropped one minute after the last request for it.
- It exists so that when the proxy container is recreated (you added a
  provider, or updated), channels that are playing carry on with the same
  session instead of being given a new one, which can show as buffering.

`<folder>_video-servers` (for example `iptv-vpn_video-servers`) is the list of
video servers the proxy has learned:

- It holds one small text file with one line per server, 200 at most. The
  lines contain server names only: no logins and no channel addresses.
- It exists so that a server seen once is already known at every later start,
  and the proxy does not have to re-read its settings while someone is
  watching.
- Nothing in it is needed to play. If it is removed, the proxy learns the
  servers again as they are used.

The relay stores nothing on disk. It keeps the newest video of each open
live channel in memory (about 15 seconds, between 12 MB and 64 MB a channel)
and is capped at 512 MB in all.

**Updating.**

- You changed `.env`: run `docker compose up -d`.
- You replaced `compose.yaml` with a newer version: run
  `docker compose up -d --force-recreate`. A plain `up -d` can leave the proxy
  running its old configuration.
- New versions of gluetun, nginx or Python: `docker compose pull`, then
  `docker compose up -d`. The images are not pinned to a version on purpose: a
  VPN client and a proxy should get their security fixes.
- Coming from the two-container version: replace `compose.yaml`, add a
  `NAME_LIVE_TV=hls` or `NAME_LIVE_TV=ts` line to every provider in `.env`
  (a provider without it is skipped), then run
  `docker compose up -d --force-recreate`.

**Removing.**

```
docker compose down -v
```

The `-v` matters: it removes both volumes too. A plain `down` leaves them
behind. Then delete the folder.

## Extras that need one more file

Two less common needs are met by a second file next to `compose.yaml`, named
`compose.override.yaml`. Docker Compose reads it automatically, so
`compose.yaml` still stays untouched.

**A VPN set up from an .ovpn file.** Put the file next to `compose.yaml`, name
it `my-vpn.ovpn`, and create `compose.override.yaml` with:

```yaml
services:
  vpn:
    volumes:
      - ./my-vpn.ovpn:/gluetun/custom.ovpn:ro
```

Then in `.env` use "Way 4" with `OPENVPN_CUSTOM_CONFIG=/gluetun/custom.ovpn`.

**A gluetun setting that is not in `.env.example`.** Add it the same way:

```yaml
services:
  vpn:
    environment:
      - SOME_GLUETUN_SETTING=value
```

## Platform notes

This is plain Docker Compose: two files in a folder and
`docker compose up -d`. That is how it was built and tested. The files are the
same everywhere; what differs is where they live and how you start them.

| Platform | Where to put the folder | How to start | Notes |
|---|---|---|---|
| Linux with Docker | anywhere, for example `/opt/iptv-vpn` | `docker compose up -d` | The reference setup. |
| Synology (Container Manager) | `/volume1/docker/iptv-vpn` | Project > Create, point it at the folder | Some models have no WireGuard in the kernel: set `WIREGUARD_IMPLEMENTATION=userspace`. |
| Proxmox | inside a VM running Docker | `docker compose up -d` | A VM is the simple route. In an LXC container you must pass `/dev/net/tun` through and allow nesting. |
| Windows (Docker Desktop with WSL 2) | a folder inside your WSL distribution | `docker compose up -d` | Use `WIREGUARD_IMPLEMENTATION=userspace`. Other devices reach it at the Windows machine's address; allow the ports in Windows Firewall. |
| Portainer | Stacks > Add stack, paste `compose.yaml` into the web editor | Deploy the stack | Under "Environment variables", use "Load variables from .env file" and pick your filled-in `.env`. |

## When something is wrong

- **Start with the status page.** Open
  `http://<the machine's address>:<any provider's port>/status` in a web
  browser. It says whether the proxy is working, names every provider that
  was left out and the line to fix, shows where the VPN comes out, and shows
  whether each provider accepted its login. If no provider could be started
  at all, the first port of `PROXY_PORTS` still answers so the page can tell
  you what is missing.
- **The status page says the relay is not answering.** Look at
  `docker logs iptv-relay`. TV keeps playing meanwhile, without the sharing.
- **A provider does not answer.** Read the `iptv-nginx` log from its last
  start. It names each provider it found and each one it skipped, with the
  reason. A provider with no `NAME_LIVE_TV` line is skipped.
- **Nothing answers.** Check that `iptv-vpn` is healthy. If the tunnel is
  down the proxy has no way out, by design.
- **A login with odd characters is refused.** Put it between single quotes in
  `.env`.
- **One channel buffers and the others are fine.** That is almost always the
  provider's server for that channel, not the proxy. In the log, each line
  shows how long the request took (the number after the size) and which
  server it came from (`uh=`). Slow lines that all name one server, while
  other channels are quick, point at that server. The same channel will be
  just as slow with the provider's login typed straight into a player.
- **Every channel buffers, on every device.** Suspect the VPN server before
  the provider. A VPN server you pick by host name can turn out to be a
  far-away entry point even when its exit shows a nearby city. A slow or
  lossy VPN server causes buffering on every device at once. Check the
  round-trip time from the machine that runs the stack to the VPN server it
  connected to (with `ping`, for example), and look for lost packets. If the
  time is high or packets go missing, choose another server.
- **Two devices cut each other off on one channel.** Check that the
  provider's `NAME_LIVE_TV` matches the live format your players really use.
  With `ts`, a channel opened as HLS while it is playing or recording as
  MPEG-TS still ends one of the two: that is the provider's rule.
- **Reading the nginx log.** `k=` is the kind of request. `s=` shows whether a
  channel's session was reused (`HIT`), opened (`MISS`) or replaced
  (`BYPASS`). `uh=` is the server the answer came from. `uct=` is how long
  it took to connect to that server, in seconds; `0.000` means an open
  connection was reused.
- **Reading the relay log.** Every line starts with `RELAY:`. It says when a
  channel was opened and closed and how many are open, how long the provider
  took to send the first video, when a reader joined or left and why, when a
  provider sent nothing for more than a few seconds and then carried on, and
  when a provider ended a channel that was being watched and the relay asked
  for it again. A channel watched as plain HLS leaves no line here: nginx
  serves it, and its requests are in the nginx log.

## How it works (technical)

This part is for engineers. Everything in it is taken from `compose.yaml`,
which holds the nginx configuration, the start script and the relay's source,
each with comments. Where this page and the file disagree, the file is right.

### Three containers, one network namespace

| Service | Container | Image | Job |
|---|---|---|---|
| `vpn` | `iptv-vpn` | `qmcgaw/gluetun` | Holds the tunnel and publishes the `PROXY_PORTS` range. |
| `nginx` | `iptv-nginx` | `nginx:alpine` | All HTTP proxying. One listener per provider. |
| `relay` | `iptv-relay` | `python:3-alpine` | Long-lived live streams, playlist rewriting, the status page. |

- `nginx` and `relay` both run with `network_mode: "service:vpn"`. They have no
  network of their own: they share the gluetun container's interfaces,
  firewall and loopback. Anything they fetch leaves through the tunnel, and if
  the tunnel is down they have no other way out.
- Because the loopback is shared, the pieces talk to each other on
  `127.0.0.1` with no Docker network between them. nginx reaches the relay on
  `127.0.0.1:8197`, the relay reaches nginx on a provider's own port, and
  nginx keeps its HLS session memory on `127.0.0.1:8195`. gluetun's control
  server is held to `127.0.0.1:8000`. None of these is published.
- The only published ports are the `PROXY_PORTS` range, on the `vpn` service.
  A provider port may not be 53, 8000, 8195, 8197 or 9999, which are in use
  inside the namespace.
- `nginx` and `relay` each wait for `vpn` to be healthy and are restarted with
  it. Each one's health check also looks for a global-scope address. If the
  tunnel container was recreated underneath it, the container is left in a
  dead namespace; the check then waits 12 seconds and stops the main process,
  so Docker starts the container again inside the tunnel's new network.
- `vpn` is given a fixed list of gluetun settings by name. `nginx` and `relay`
  read the whole settings file (`.env`, or `stack.env` under Portainer), and
  each one's start command strips the environment down before the real
  program runs: nginx starts with `PATH` and `TZ` only, the relay with `PATH`
  and a list of provider addresses.

### What nginx does

**At start.** The entrypoint script finds every `NAME` that has `NAME_URL` in
the environment and checks it: all five lines present, a port inside
`PROXY_PORTS` that is not reserved and not used twice, an address of the form
`http(s)://name[:port]`, and `NAME_LIVE_TV` equal to `hls` or `ts`. For each
good one it writes a `server` block with the listen port, the provider
address, the percent-encoded login and the live format, and includes the
shared locations. For each bad one it prints `PROVIDER NAME SKIPPED:` with the
reason. The same findings are written to a small JSON file that nginx serves
to loopback callers only, which is where the status page gets them. If no
provider could be started, one listener on the first port of the range still
serves `/status`, and the container reports itself unhealthy.

**Proxying.** HTTP/1.1 to the upstream, response buffering off so bytes are
passed on as they arrive, a 10 second connect timeout and a 60 second limit
on the gap between bytes (not on stream length). Upstream certificates are
not verified, which is nginx's default: many players do not check them
either, and some providers' servers carry expired or self-signed ones.

**Redirects and addresses.** A `Location` naming another server is rewritten
to `/__cdn/<scheme>/<host>/...` on the proxy, so the player comes back and the
fetch goes through the tunnel. A `/__cdn/` target may not be `localhost`, a
loopback, private, link-local or carrier-grade NAT address, a bracketed IPv6
literal, a host containing `@` or `%`, or a numeric address written with
leading zeros. The same list is applied to the redirect a live HLS channel is
sent to. Picture addresses in `player_api.php` answers are rewritten to the
same `/__cdn/` form. `get.php` is refused with 403.

**The login swap.** A request that carries `PLAYER_USER` and `PLAYER_PASSWORD`
in the path or the query string is sent upstream with the provider login of
the port it arrived on. Nothing else in the address is changed. Any other
login is passed on untouched and refused by the provider.

**Session memory for live HLS.** An internal server on `127.0.0.1:8195` is an
nginx proxy cache that stores only redirect answers, keyed by provider and
channel path. Opening a channel asks that cache where the provider sent it,
then hands the player a folder of its own, `/__ch/<channel address>/`. Every
later refresh of the playlist, and every piece named by bare file name, is
resolved against the remembered redirect, so all readers of a channel stay on
one provider session. Opens that arrive together wait for the first one (cache
lock, up to 20 seconds). An entry is dropped one minute after its last use.
When the remembered session answers with an error, the channel is opened
again once, bypassing the cache, and the new redirect is remembered. The
cache directory is a Docker volume so that playing channels keep their
session when the nginx container is recreated.

**Connection reuse and learned video servers.** nginx keeps upstream
connections open only to servers it has an `upstream` block for, and no
server is named in the file. So a log line is written to a small file for
every request answered with 200 or 206 by a server reached by name on port
443 with a video or playlist content type. A loop in the entrypoint reads
that file every 5 seconds. When it sees a new name it waits 20 seconds for
channel changes to settle, writes one `upstream` block per new server (8 idle
connections, each let go after 20 seconds without use, `max_fails=0` so a server is never set
aside), tests the configuration and reloads nginx once. The list is saved to
a second Docker volume, read back at start through a strict pattern, and
capped at 200 entries.

**One worker.** The entrypoint sets `worker_processes 1`. The image default is
one worker per CPU thread, and each worker has its own pool of upstream
connections, so a stream that bounced between workers kept opening new ones.
A home proxy carries a handful of streams, and one worker reuses the same
connections for all of them.

**Logging.** One line per request with the port, status, sizes, timings, user
agent, kind of request, session-memory result, upstream host and connect
time. The request address is never logged, because Xtream addresses contain
the login. nginx's error log is limited to critical messages for the same
reason.

**What it hands to the relay.** Three kinds of request are proxied to
`127.0.0.1:8197`: a live MPEG-TS request made with the easy login (to `/ts`
or `/hls`, according to that provider's `NAME_LIVE_TV`), a `GET` for a
playlist under `/__ch/`, `/__cdn/` or `/__cdu/` (to `/pl`), and `/status`.
nginx tells the relay what it needs in `X-Relay-*` request headers. On every
request handed to the relay, nginx sets or blanks all of those headers
itself, and the `X-Hls-*` ones used by the session memory are blanked on
every request that does not use it. So the relay and the session memory
never see a value sent by a player.

### What the relay does

The relay is one asyncio program. It listens on `127.0.0.1:8197`, so nginx is
its only caller. It has four endpoints that matter, plus `/healthz` for the
container's health check and `/stat`, a one-line count of open channels and
readers.

| Endpoint | Job |
|---|---|
| `/ts` | Share one provider MPEG-TS stream between every reader of a channel. |
| `/hls` | Give a reader that asks for MPEG-TS the channel's HLS video, joined end to end. |
| `/pl` | Point the addresses inside an HLS playlist back at the proxy. |
| `/status` | Draw the status page. |

For `/ts` and `/hls`, nginx sends the provider's address, the channel's
address at the provider (with the provider login already swapped in), the
address as the player sent it, and the port. The relay answers 403 unless the
provider is one of the addresses it was started with. A `HEAD` request is
answered with 200 on the spot, because asking the provider would open a
connection. One channel has one key whichever form of its address a player
uses (`/live/user/pass/1.ts`, `/user/pass/1.ts`, `/user/pass/1`). The relay
serves at most 100 readers at once, all channels together.

### MPEG-TS sharing (`/ts`)

- **One upstream per channel.** The first reader of a channel makes the relay
  open it at the provider, with that player's own request headers. Every
  later reader of the same channel is attached to the same upstream. The
  provider sees one connection however many readers there are, and one
  reader leaving does not disturb the rest.
- **A ring of recent video.** For each open channel the relay keeps the
  newest video in memory: about 15 seconds of it, never less than 12 MB and
  never more than 64 MB.
- **Where a joiner starts.** The reader that opens a channel gets the stream
  from its first byte. A reader that joins later is handed the newest 10
  seconds of video at once, so it starts about 10 seconds behind live and has
  something in hand when the provider pauses, as a player on its own
  connection has from the provider's opening burst. The amount is counted in
  seconds, not bytes: the relay measures each channel's real rate over the
  last minute and over the last 15 seconds, takes the larger, and multiplies.
  It never hands over more than the last 20 seconds of arrivals. In a channel's first 8 seconds there is no rate
  yet, and a joiner gets everything kept so far. The joiner's first byte is
  placed on a program table (PAT) packet if one is near, otherwise on the
  first whole packet.
- **Slow readers are cut off.** Each reader has its own queue. A reader that
  has been more than 16 MB behind its channel for 15 seconds, or is ever 64
  MB behind, is disconnected, so that what is held for it stays bounded. What
  a joiner was handed at the start is not counted against it, until it has
  been sent on. This is also
  how long a paused player is waited for.
- **Last reader out.** When the last reader leaves, the provider's connection
  is dropped at once. The exception is a channel in its first 5 seconds,
  where a tuning player often hangs up and asks again within a moment: the
  channel lingers for 0.4 seconds unless that player has just opened another
  channel.
- **Changing channel.** A player hangs up on the old channel and asks for the
  new one at almost the same moment, and the two can arrive in either order.
  Before a new channel is opened, the relay drops that provider's channels
  that nobody is reading, gives a hang-up that is just behind the request a
  moment (30 ms) to arrive, and leaves the provider 0.1 seconds after the
  drop to notice it. If the player has hung up by then, nothing is opened.
  The aim is that a one-connection account never sees two connections during
  a channel change.
- **A stream the provider ends.** If the provider ends a stream, or it sends
  nothing for 60 seconds, while readers are still attached, the relay asks
  for the channel again after 0.1 seconds and carries on, so the players do
  not each have to notice and reconnect. It does this only for a stream that
  had run for at least 30 seconds, and not when another channel of the same
  provider was opened in the last 15 seconds (on a one-connection account
  that is why it ended). Asked again, the provider gets 5 seconds to answer
  and 10 to send video; after that the readers are let go, as they would be
  on a connection of their own. A channel that
  goes silent twice in a row, each time within 90 seconds of being asked for,
  is treated as dead: the readers are let go and the provider is not asked
  again. The new stream is joined on at a clean packet, and a
  packet the old stream left half-sent is filled out first so the packets
  stay aligned.
- **Refusals.** If the provider refuses the channel, its status code and the
  body of its answer are passed to the player. A reader waits up to 60
  seconds for a channel's first video.

### The HLS-to-MPEG-TS bridge (`/hls`)

With `NAME_LIVE_TV=hls`, a live `.ts` request does not go to the provider as
MPEG-TS. The relay builds the address of the same channel as HLS on this
proxy, `/__ch/<channel>.m3u8/__index.m3u8` on the provider's port, and reads
it from nginx on loopback, exactly as a player would. That puts the reader on
the session memory described above, so it shares the one provider session
with every HLS player on the channel.

- It reads the playlist, follows a list of qualities to the highest
  `BANDWIDTH` on offer, and starts near the newest video (the last three
  pieces).
- It fetches each new piece once, in order, and sends the pieces end to end
  as one MPEG-TS stream. It asks for the playlist again at the playlist's
  target duration, or half of it when nothing was new.
- Readers of the same bridged channel share one such feed, with the same ring
  and joiner rules as `/ts`.
- A piece that cannot be fetched after two tries is skipped and noted in the
  log; three in a row end the stream. A playlist that cannot be had is asked
  for again (4 tries at the start, 9 once playing).
- It only joins playlists made of whole, unencrypted MPEG-TS files. A
  playlist with `EXT-X-MAP`, `EXT-X-BYTERANGE` or an `EXT-X-KEY` other than
  `METHOD=NONE` cannot be joined end to end and is not bridged.
- Every address it fetches must be this proxy's own. A playlist line that
  leads anywhere else is not fetched.
- The stream ends when the playlist carries `EXT-X-ENDLIST` or stops moving
  for 30 seconds (or six target durations, if that is longer).

The bridged stream is the HLS one, so it runs a few seconds further behind
live than the provider's own MPEG-TS would.

### Playlist rewriting (`/pl`)

nginx has always rewritten playlists with text rules (`sub_filter`). Those
rules are still there, and the relay now does the job first because it reads
a playlist line by line, the way a player does, and so copes with forms that
text matching would miss.

1. A player's `GET` for a playlist is handed to the relay with the original
   address and port.
2. The relay asks nginx for that same address from the source address
   `127.0.0.2`. For that one caller nginx fetches the playlist from the
   provider as usual (same session, same kept connections) but its text rules
   put back exactly what they found, so the playlist arrives as the provider
   wrote it. nginx adds three response headers that say where this
   playlist's addresses lead: the proxy path for the server it came from, the
   same for scheme-relative addresses, and the proxy path for the playlist's
   own folder one level up.
3. The relay rewrites every address line and every `URI="..."` inside a tag:
   full `http://` and `https://` addresses, `//server/path`, `/path` and
   `../path`. An address with none of those beginnings sits next to the
   playlist and is left as it is. Bytes outside addresses come out as they
   went in. The player's request headers and the server's response headers
   are passed through, apart from per-connection ones, range headers,
   `Server` and `Date` (nginx adds its own), and the proxy's own notes.
4. nginx strips those three headers from anything an upstream sends, so only
   nginx can set them and a server cannot use them to point a player
   somewhere else.

**The fallback.** If the relay does not answer (it is not running, or it
steps aside because the body is not a playlist, is over 1 MB or has more than
20,000 lines), nginx gets a failed upstream and serves the request itself: it
re-requests the same address from `127.0.0.3`, a caller for which the text
rules work as they always did. A playlist that arrives compressed, which
should not happen because nginx asks for plain text, cannot be read by the
relay. It steps aside in the same way and nginx serves the playlist itself,
as it came.

### The status page (`/status`)

nginx proxies `/status` on any provider port to the relay, with the port and
the easy login in headers. The relay then:

- reads the start-up notes from nginx (what was found, what was skipped and
  why)
- asks `https://ipinfo.io/json` for the public address, through the tunnel,
  with certificate checking on, at most once every 5 minutes (once a minute
  while the lookup is failing)
- asks each provider for its account details through nginx, with the easy
  login, so the request takes the same path a player's would. This happens when
  someone views the page, at most once every 15 seconds, and the page waits up
  to 6 seconds for the answer so that what it shows is current. Each answer is
  labeled with its age ("Checked just now"), and the page reloads itself every
  30 seconds. If the provider is slow, the last good answer is shown with its
  age, for up to 15 minutes. After a failure the provider is asked less and
  less often (up to 15 minutes apart).
- counts live channels: HLS channels whose playlist was asked for in the last
  30 seconds (by a player, or by the relay for a recorder), MPEG-TS channels and their readers, and MPEG-TS readers
  being fed from HLS. Movies, series and catch-up are not counted.

The page returns 200 when at least one provider is set up and there is no
settings problem, and 503 otherwise. It shows no logins.

### When the relay is not running

| Request | What nginx does instead |
|---|---|
| Live MPEG-TS | Passes it to the provider directly, each reader on a connection of its own. |
| A playlist | Rewrites it with its own text rules. |
| `/status` | Answers 503 with one line that says to look at the relay's log. |

Everything else (the API, the guide, movies, series, catch-up, HLS pieces,
the session memory, the login swap) never goes through the relay.

### How the relay is fenced in

As written in `compose.yaml`:

- **Network.** It shares the tunnel's namespace and listens on loopback only.
  It connects to the providers named in `.env` and the servers they redirect
  to, to nginx in this stack, and to `ipinfo.io` for the status page. Before
  it follows a redirect it looks the name up itself, refuses a private,
  loopback, link-local or carrier-grade NAT address, and connects to the very
  address it checked. Its fetches from nginx are never allowed to be
  redirected to another host.
- **What it is given from `.env`.** The start command works out which
  providers are set up, using the main checks nginx uses (the name, the
  address form, the lines being present, `hls` or `ts`), and starts Python
  with a cleared environment holding only `PATH` and the list of those
  providers' addresses. No login, password or VPN key is in the program's
  environment. It does see a provider's login when nginx hands it a live
  channel's address, and the easy login when the status page is drawn; it
  keeps neither on disk.
- **User.** It runs as `65534:65534` (nobody), with no added capabilities and
  no volumes.
- **Memory.** `mem_limit: 512m`, a ceiling in case a provider misbehaves.
- **Health check.** Every 30 seconds: it must still have an address in the
  tunnel's namespace, and `/healthz` on `127.0.0.1:8197` must answer.
- **Logs.** Its log lines never contain an address or a login.
- **Dependencies.** None. The script uses only the Python standard library
  and refuses to start on a Python older than 3.12.

### Why Python, and why not inside nginx

The design rule is that nginx keeps doing what it is good at, which is HTTP
proxying, and nothing else is added unless plain nginx settings cannot do the
job. Four things could not be done with settings, or not well enough:
fanning one upstream stream out to several readers, joining HLS pieces into
one stream, reading a playlist line by line, and drawing a status page.

The first attempt, which was never published, did the stream work inside
nginx with its JavaScript module (njs). Measured on real streams, it either
grew in memory for as long as video flowed through it, or, when that was
worked around, corrupted the stream. Neither is acceptable in the video
path. So the long-lived stream handling moved out to a small asyncio program:
one process, standard library only, in the official Python image, with its
source inline in the compose file like the nginx settings. nginx still
terminates every player connection and still does all proxying to providers
for everything that is not a shared live stream.

### Limits, stated plainly

- **MPEG-TS smoothness depends on how evenly the provider sends video.** The
  relay passes video on as it arrives and does not pace it. A joiner has
  about 10 seconds in hand. If a provider pauses for longer than a player has
  buffered, that player stalls, as it would on a direct connection.
- **HLS is the recommended setting where a provider offers it.** With HLS the
  player fetches finished pieces and keeps its own buffer, nginx's session
  memory survives a recreate of the nginx container, and a recorder that
  wants MPEG-TS is still served through the bridge.
- **With `ts`, HLS and MPEG-TS do not mix on one channel.** Opening a channel
  as HLS while it is being read as MPEG-TS still ends one of the two. That is
  the provider's rule, and turning MPEG-TS into HLS is beyond this proxy.
- **The bridge needs plain HLS.** Encrypted or byte-range playlists, and ones
  with an `EXT-X-MAP` line, are not bridged.
- **Recreating the relay container ends the live streams that run through
  it.** That happens when `.env` changes or the stack is updated.
- **Certificates of providers are not verified**, by nginx or by the relay,
  for the reason given above.
- **The relay's parts are newer than the rest.** See
  [What was tested, and how](#what-was-tested-and-how).

## Support

There is none. This was made for personal use and is shared as is, free, in
case it is useful to someone else. Issues and pull requests are turned off,
and questions will not be answered. Everything needed to use it is on this
page. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE). gluetun, nginx and Python are separate projects with their own
licenses. This repository contains configuration for them and one small
Python program, and does not distribute any of the three.
