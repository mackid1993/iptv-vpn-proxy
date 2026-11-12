# iptv-vpn-proxy

Put all of your IPTV behind one VPN, on every device, without installing a VPN
on any of them.

Two containers run on a machine at home. Your players connect to that machine
instead of to your IPTV provider, and everything they fetch leaves through the
VPN. On the phone, TV or streaming box you type an address and a login into
your player, the same way you would with the provider's own details. That is
the whole setup on the device.

- **gluetun** holds the VPN tunnel.
- **nginx** sits inside that tunnel and passes your players' requests on to the provider.

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
- [How it works](#how-it-works)
- [What was tested, and how](#what-was-tested-and-how)
- [Trade-offs and things to know](#trade-offs-and-things-to-know)
- [What it stores, updating, removing](#what-it-stores-updating-removing)
- [Extras that need one more file](#extras-that-need-one-more-file)
- [Platform notes](#platform-notes)
- [When something is wrong](#when-something-is-wrong)
- [Support](#support)
- [License](#license)

## Design goals

1. **Parity with direct credentials.** A player pointed at the proxy should
   behave exactly as if you had typed the provider's own Xtream login into it.
   Not better, not different: the same, with the VPN added.
2. **One VPN for everything.** Every device and every player goes out through
   the same tunnel, and no device needs a VPN app.
3. **The easiest setup that can work.** One settings file. You never edit the
   compose file. Adding a provider is four lines.
4. **Works with any player and any Xtream provider.** Nothing in it is written
   for one provider or one app.
5. **No custom software.** It is stock gluetun and stock nginx, configured. The
   only script is a short shell loop inside the compose file that reads your
   settings at start.

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

## What you need

- A machine at home that is always on, with Docker and Docker Compose 2.24 or newer.
- A VPN that gluetun supports, or a WireGuard or OpenVPN config file from any VPN.
- IPTV providers that give you an **Xtream login**: a server address, a username and a password.

## Set it up

1. Make a folder and put `compose.yaml` and `.env.example` in it. Both are in
   this repository, and in full [below](#the-two-files).
2. Copy `.env.example` to `.env`.
3. Fill in `.env`. It has four sections and explains each one:
   your VPN, the easy login you choose, the port range, and your providers.
4. Start it:
   ```
   docker compose up -d
   ```
5. Check the log of the `iptv-nginx` container. Each start lists what it found:
   ```
   PROVIDER PROVIDER1 on port 8095 -> https://provider.example
   ```
   A provider with a problem is listed as `SKIPPED` with the reason, and the
   others still start.
6. In your player or playlist manager, add an **Xtream** source:
   - server: `http://<the machine's address>:<that provider's port>`
   - username and password: the `PLAYER_USER` and `PLAYER_PASSWORD` you chose

Each provider gets its own port. The same easy login works for all of them.

## The two files

These are the same files that sit in this repository, shown here so you can
read them without leaving the page. Click a heading to open it.

### `.env.example`: every setting, with instructions

Copy it to `.env` and fill it in. This is the only file you edit. It covers the
VPN, the login your players use, the ports, and one block of four lines per
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
#   4. one block of four lines per IPTV provider
#
# You never edit compose.yaml. Change this file, then bring the stack up
# again ("docker compose up -d", or the Up / Deploy / Build button of your
# Docker manager). Compose notices the change and restarts what needs it:
#   - a change to a provider or to the easy login restarts only the proxy
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
# The settings shown in this section are the ones this stack passes on to
# gluetun. The README says how to add a different one.
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
# next to compose.yaml; the README shows it ("OpenVPN from an .ovpn file"). The login lines are only needed if
# the file asks for one.
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
# FIREWALL_OUTBOUND_SUBNETS is for the other direction: networks the two
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
# Each provider is a block of four lines that share one NAME:
#
#     NAME_URL=https://provider.example      the server address the provider
#                                            gave you: http:// or https://,
#                                            the server name, optionally
#                                            :port, and nothing after that
#     NAME_PORT=8095                         a free port inside PROXY_PORTS
#     NAME_USER=the-username-they-gave-you
#     NAME_PASSWORD=the-password-they-gave-you
#
# NAME is any word you like in CAPITALS (letters, digits, _), for example
# PROVIDER1 or SPORTS. It is only a label; all four lines must use the same
# one.
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
#   1. Copy the example block below, remove the # in front of its four
#      lines, change NAME, and fill in the values.
#   2. Bring the stack up again.
#   3. Check it was picked up: the iptv-nginx container's log lists the
#      providers found at every start, like
#          PROVIDER PROVIDER1 on port 8095 -> https://provider.example
#      A provider with a problem is listed as SKIPPED with the reason, and
#      the other providers keep working.
#   4. In your player or playlist manager add an Xtream source with
#          server    http://<this machine's address>:<NAME_PORT>
#          username  the PLAYER_USER above
#          password  the PLAYER_PASSWORD above
#
# TO REMOVE A PROVIDER: delete its four lines (or put # in front of them)
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

# ---------- a second one ----------
# PROVIDER2_URL=http://another-provider.example:8080
# PROVIDER2_PORT=8096
# PROVIDER2_USER=the-username-they-gave-you
# PROVIDER2_PASSWORD='the-p@ssword-they-gave-you'

```

</details>

### `compose.yaml`: the stack itself

You do not edit this one. It defines the two containers and holds the whole
nginx configuration, with a comment on each part saying what it is for.

<details>
<summary><b>Show compose.yaml</b></summary>

```yaml
##############################################################################
# IPTV through a VPN, with nothing to install on your devices
#
#   iptv-vpn     gluetun. Holds the VPN tunnel (WireGuard or OpenVPN, any
#                provider gluetun supports).
#   iptv-nginx   A reverse proxy in front of your IPTV providers. It lives
#                inside the tunnel's network, so everything it fetches from a
#                provider goes out through the VPN and nothing else can.
#
# Your players and playlist managers connect to this machine instead of to
# the provider:
#        http://<this machine>:<the provider's port>
#        username / password = PLAYER_USER / PLAYER_PASSWORD from .env
# and behave as if the provider's own Xtream login had been typed into them.
#
# YOU NEVER EDIT THIS FILE. Every setting is in .env (copy .env.example to
# .env and fill it in). To add a provider, add four lines to .env:
#        NAME_URL=https://provider.example
#        NAME_PORT=8095
#        NAME_USER=...
#        NAME_PASSWORD=...
# then bring the stack up again. The iptv-nginx log lists the providers it
# found at each start, and names any it skipped and why.
#
# What the proxy does besides passing bytes on (each part is explained where
# it sits in the config at the bottom of this file):
#   1. keeps redirects inside the tunnel
#   2. refuses to be pointed at this machine or your home network
#   3. points the pieces of an HLS playlist back at itself
#   4. keeps ONE provider session per live HLS channel
#   5. swaps the easy login for the provider's
#
# Works with HLS, MPEG-TS, movies, series, catch-up and the guide.
#
# FOR YOUR HOME NETWORK ONLY. The proxy ports have no protection of their
# own. Never forward them from the internet; reach them from outside with
# your own private network (Tailscale, WireGuard, ...) instead.
#
# Good to know:
#   - Xtream logins only. A provider's ready-made M3U link (get.php) is
#     refused, because the addresses inside it skip the VPN.
#   - When a player logs in, the provider's reply includes the provider's own
#     server address, and the proxy passes that reply on unchanged. A player
#     that connects to that address instead of the one you typed would skip
#     the VPN.
#   - Channel logos and posters are fetched by the player straight from
#     wherever they are hosted, not through the VPN. Only pictures.
##############################################################################

services:

  # ---------------------------------------------------------------------------
  # The VPN tunnel. Anything sharing this container's network has no other way
  # out, so if the tunnel is down nothing leaks. All of its settings are
  # gluetun's own, read from .env (see the notes there).
  # ---------------------------------------------------------------------------
  vpn:
    image: qmcgaw/gluetun:latest
    container_name: iptv-vpn
    restart: unless-stopped
    cap_add:
      - NET_ADMIN
    devices:
      - /dev/net/tun:/dev/net/tun
    ports:
      # Every provider port, same number inside and out. One range covers all
      # providers, present and future. The range is PROXY_PORTS in .env: set it
      # to whatever ports are free on your machine.
      - "${PROXY_PORTS:-8095-8096}:${PROXY_PORTS:-8095-8096}"
    # The VPN settings, taken from .env by name. A setting you leave out of
    # .env is simply not passed on, and gluetun uses its own default. Only a
    # change to one of these (or to PROXY_PORTS) restarts the tunnel; adding or
    # changing an IPTV provider does not.
    environment:
      - TZ
      - VPN_SERVICE_PROVIDER
      - VPN_TYPE
      - SERVER_COUNTRIES
      - SERVER_REGIONS
      - SERVER_CITIES
      - SERVER_HOSTNAMES
      - WIREGUARD_PRIVATE_KEY
      - WIREGUARD_ADDRESSES
      - WIREGUARD_PRESHARED_KEY
      - WIREGUARD_PUBLIC_KEY
      - WIREGUARD_ENDPOINT_IP
      - WIREGUARD_ENDPOINT_PORT
      - WIREGUARD_MTU
      - WIREGUARD_PERSISTENT_KEEPALIVE_INTERVAL
      - WIREGUARD_IMPLEMENTATION
      - OPENVPN_USER
      - OPENVPN_PASSWORD
      - OPENVPN_PROTOCOL
      - OPENVPN_CUSTOM_CONFIG
      - FIREWALL_OUTBOUND_SUBNETS
      - DNS_UPSTREAM_RESOLVER_TYPE
      - DNS_UPSTREAM_RESOLVERS
      - DNS_UPSTREAM_PLAIN_ADDRESSES
      - BLOCK_MALICIOUS
      # gluetun's own control page stays inside the container.
      - HTTP_CONTROL_SERVER_ADDRESS=127.0.0.1:8000

  # ---------------------------------------------------------------------------
  # nginx. Shares the tunnel's network. Its whole configuration is the text
  # under "configs:" at the bottom of this file.
  # ---------------------------------------------------------------------------
  nginx:
    image: nginx:alpine
    container_name: iptv-nginx
    restart: unless-stopped
    network_mode: "service:vpn"         # all its traffic goes through the tunnel
    depends_on:
      vpn:
        condition: service_healthy
        restart: true                   # comes back with the tunnel
    # The provider lines are read from your settings file. That file is .env;
    # Portainer saves the one you upload as stack.env. Whichever exists is used.
    env_file:
      - path: .env
        required: false
      - path: stack.env
        required: false
    volumes:
      # Where the proxy remembers each channel's session. Kept in a small Docker
      # volume so that when this container is recreated (a provider was added,
      # an update), channels that are playing carry on with the same session.
      - hls-memory:/var/cache/nginx/hls
    configs:
      - source: proxy
        target: /etc/nginx/conf.d/default.conf
      - source: provider-locations
        target: /etc/nginx/provider-locations.conf
    # At every start: build one nginx server block per provider found in .env
    # (any NAME with NAME_URL, NAME_PORT, NAME_USER and NAME_PASSWORD), then
    # start nginx as usual. A provider with a missing or unusable line is
    # skipped and named in the log; the others still start.
    #
    # Provider logins may contain ANY characters. Each one is percent-encoded
    # here (letters, digits and - . _ ~ stay as they are; everything else
    # becomes %XX), which is the form a URL needs, so nginx only ever sees
    # safe text and the provider receives exactly the login it issued.
    entrypoint:
      - /bin/sh
      - -c
      - |
        out=/etc/nginx/conf.d/providers.conf
        : > "$$out"
        enc() {
          printf '%s' "$$1" | hexdump -v -e '1/1 "%02x\n"' | while read -r h; do
            case "$$h" in
              2d|2e|5f|7e|3[0-9]|4[1-9a-f]|5[0-9a]|6[1-9a-f]|7[0-9a]) printf "\\x$$h" ;;
              *) printf '%%%s' "$$(echo "$$h" | tr a-f A-F)" ;;
            esac
          done
        }
        ports="$${PROXY_PORTS:-8095-8096}"; stop=""
        echo "$$ports" | grep -Eqx '[1-9][0-9]{0,4}(-[1-9][0-9]{0,4})?' || stop="PROXY_PORTS must be one port or a range like 8095-8099 (found: $$ports)"
        [ -z "$$stop" ] && { echo "$$PLAYER_USER" | grep -Eqx '[A-Za-z0-9_-]+' && echo "$$PLAYER_PASSWORD" | grep -Eqx '[A-Za-z0-9_-]+' || stop="PLAYER_USER and PLAYER_PASSWORD must be letters, digits, - or _ only (they are yours to choose)"; }
        [ -z "$$stop" ] && case "$$PLAYER_USER" in live|movie|series|timeshift) stop="PLAYER_USER cannot be live, movie, series or timeshift";; esac
        lo=$${ports%-*}; hi=$${ports#*-}; used=" "
        [ -n "$$stop" ] && echo "PROXY SETTINGS PROBLEM: $$stop"
        env | sed -n 's/^\([A-Za-z0-9_]*[a-z][A-Za-z0-9_]*\)_URL=.*/\1/p' | while read -r low; do echo "PROVIDER $$low SKIPPED: the name must be in CAPITALS"; done
        [ -z "$$stop" ] && for name in $$(env | sed -n 's/^\([A-Z][A-Z0-9_]*\)_URL=.*/\1/p' | sort); do
          url=$$(printenv "$${name}_URL"); port=$$(printenv "$${name}_PORT")
          user=$$(printenv "$${name}_USER"); pass=$$(printenv "$${name}_PASSWORD")
          bad=""
          case "$$name" in PROXY|PLAYER|VPN|OPENVPN|WIREGUARD|SERVER|FIREWALL|DNS) bad="$$name is one of the proxy's own names; pick another";; esac
          [ -z "$$bad" ] && { [ -n "$$url" ] && [ -n "$$port" ] && [ -n "$$user" ] && [ -n "$$pass" ] || bad="needs $${name}_URL, _PORT, _USER and _PASSWORD"; }
          [ -z "$$bad" ] && { echo "$$port" | grep -Eqx '[1-9][0-9]{0,4}' && [ "$$port" -le 65535 ] || bad="port must be a number from 1 to 65535"; }
          [ -z "$$bad" ] && { echo "$$url" | grep -Eqx 'https?://[A-Za-z0-9._-]+(:[0-9]{1,5})?/?' || bad="address must be http://name or https://name, optionally :port, with nothing after it"; }
          [ -z "$$bad" ] && case " 53 8000 8195 9999 " in *" $$port "*) bad="port $$port is used inside the VPN container itself";; esac
          [ -z "$$bad" ] && case "$$used" in *" $$port "*) bad="port $$port is already used by another provider";; esac
          [ -z "$$bad" ] && { [ "$$port" -ge "$$lo" ] && [ "$$port" -le "$$hi" ] || bad="port $$port is outside PROXY_PORTS ($$ports)"; }
          if [ -n "$$bad" ]; then echo "PROVIDER $$name SKIPPED: $$bad"; continue; fi
          used="$$used$$port "
          printf 'server {\n  listen %s;\n  set $$provider  %s;\n  set $$pscheme   %s;\n  set $$acct_user "%s";\n  set $$acct_pass "%s";\n  include /etc/nginx/provider-locations.conf;\n}\n' \
            "$$port" "$${url%/}" "$${url%%://*}" "$$(enc "$$user")" "$$(enc "$$pass")" >> "$$out"
          echo "PROVIDER $$name on port $$port -> $${url%/}"
        done
        [ "$$used" = " " ] && echo "NO PROVIDERS STARTED. nginx is running with none: fix .env and bring the stack up again."
        # nginx itself needs none of the settings (they are in the file written
        # above), so it starts without them.
        exec env -i PATH="$$PATH" TZ="$${TZ:-UTC}" /docker-entrypoint.sh nginx -g 'daemon off;'

    # Healthy = nginx is still inside the tunnel container's network AND it
    # answers. If the tunnel container was restarted underneath nginx, nginx is
    # left in a dead network with no address of its own; then the check
    # waits a moment for the tunnel container to be back and stops nginx, so
    # Docker starts it again inside the tunnel's new network.
    healthcheck:
      test: ["CMD-SHELL", "ip -o addr show scope global | grep -q . || { sleep 12; kill 1; exit 1; }; wget -q -T 5 -O /dev/null http://127.0.0.1:8195/healthz || exit 1"]
      interval: 30s
      timeout: 20s
      start_period: 20s
      retries: 3

volumes:
  hls-memory:

configs:
  proxy:
    content: |
      # Names are looked up through the tunnel container's own DNS.
      resolver 127.0.0.1 ipv6=off valid=60s;
      resolver_timeout 5s;

      # ---- Logging. Xtream addresses contain the login, so the address is never logged.
      map $$uri $$kind {
        default                              api;
        ~^/__cdn/.*\.m3u8$$                   cdn-playlist;
        ~^/__cdn/                            cdn-media;
        ~^/__ch/.*\.m3u8$$                    hls;
        ~^/__ch/                             hls-media;
        ~\.m3u8$$                             hls-open;
        ~^/(live|movie|series|timeshift)/    media;
        /xmltv.php                           guide;
      }
      log_format masked '$$remote_addr [$$time_local] $$server_port $$status $$body_bytes_sent $$request_time done=$$request_completion up=$$upstream_status up_bytes=$$upstream_bytes_received up_time=$$upstream_response_time m=$$request_method rng="$$http_range" ua="$$http_user_agent" k=$$kind s=$$sess uh=$$proxy_host';
      error_log /dev/stderr crit;                # upstream error lines would print the address, login included

      # ---- Plain reverse-proxy settings, the same for everything.
      proxy_http_version 1.1;
      proxy_set_header Connection "";
      proxy_set_header Accept-Encoding "";      # plain text, so HLS playlists can be read (see sub_filter)
      proxy_set_header X-Hls-Provider "";       # internal notes of the session memory; never accepted from a player
      proxy_set_header X-Hls-Renew "";
      proxy_ssl_server_name on;
      # Certificates are not checked (nginx's default). Many players do not check
      # them either, and some providers' servers have expired or self-made ones;
      # checking here would refuse servers that play fine without the proxy.
      proxy_buffering off;                      # pass bytes on as they arrive
      proxy_request_buffering off;
      proxy_max_temp_file_size 0;
      proxy_buffer_size 64k;
      proxy_buffers 8 64k;
      chunked_transfer_encoding off;            # end streams by closing, like the provider does
      proxy_connect_timeout 10s;
      proxy_read_timeout 60s;                   # longest gap between bytes, not stream length
      send_timeout 60s;

      # ---- 1. Redirects (MPEG-TS, movies, series, catch-up, the guide). A provider
      # ---- answers "go to that other server". Rewrite the address so the player
      # ---- comes back to this proxy for it: /__cdn/<scheme>/<host>/...
      absolute_redirect off;                    # hand the player "/__cdn/..." as is, never "http://this-host/__cdn/..."
      proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
      proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$pscheme/$$1$$2;   # "//server/path": same http or https as the provider

      # ---- 2. Where a /__cdn/ request really goes, and what it may not reach.
      map $$request_uri $$cdn_target {
        default "";
        ~*^/__cdn/(?<cs>https?)/(?<ch>[^/?]+)(?<cr>/.*)$$  $$cs://$$ch$$cr;
        ~*^/__cdn/(?<cs>https?)/(?<ch>[^/?]+)$$            $$cs://$$ch/;
      }
      map $$request_uri $$cdn_prefix {
        default "";
        ~*^(?<cp>/__cdn/https?/[^/?]+)  $$cp;
      }
      map $$request_uri $$cdn_scheme {
        default http;
        ~*^/__cdn/https/  https;
      }
      map $$request_uri $$cdn_forbidden {         # nothing on this machine or this network
        default 0;
        "~*^/__cdn/https?/(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|\[|[^/]*[@%])"  1;
      }

      # ---- 3. HLS playlists name their video pieces without a server (or with the
      # ---- other server's full address). Point those lines back at this proxy too.
      sub_filter_types application/vnd.apple.mpegurl application/x-mpegurl application/mpegurl audio/mpegurl audio/x-mpegurl;
      sub_filter_once off;
      sub_filter "\n/"            "\n$$cdn/";
      sub_filter 'URI="/'         'URI="$$cdn/';
      sub_filter "\nhttps://"     "\n/__cdn/https/";
      sub_filter "\nhttp://"      "\n/__cdn/http/";
      sub_filter 'URI="https://'  'URI="/__cdn/https/';
      sub_filter 'URI="http://'   'URI="/__cdn/http/';

      # ---- 4. Live HLS: one provider session per channel.
      # ---- Many providers start a NEW session every time a channel's address is
      # ---- asked, and a new session ends the one before it. A player refreshes
      # ---- that address every few seconds, and several players or a recording may
      # ---- be on the same channel. So the proxy remembers where the provider sent
      # ---- a channel and keeps everyone on that session: the pieces keep the same
      # ---- addresses from one refresh to the next, and nobody ends anyone else's
      # ---- stream. When the remembered session is gone, it is replaced once.
      proxy_cache_path /var/cache/nginx/hls levels=1 keys_zone=hls:1m max_size=16m inactive=1m use_temp_path=off;

      server {                                  # the memory itself; reachable only from this container
        listen 127.0.0.1:8195;
        access_log off;
        location = /healthz { return 200; }     # the container's health check
        location / {
          set $$cdn "";
          set $$sess "-";                       # (these three keep the config valid when no provider is set up)
          set $$pscheme http;
          proxy_redirect off;
          proxy_buffering on;                   # needed to remember an answer; this one is only a redirect
          proxy_cache hls;
          proxy_cache_key "$$http_x_hls_provider$$uri";   # the channel address; a query string never names another channel
          proxy_cache_valid 301 302 303 307 308 365d;  # only the "it is over there" answer is remembered, for as long as the channel is in use
          proxy_cache_lock on;                  # opens arriving together wait for the first one
          proxy_cache_lock_timeout 5s;
          proxy_cache_bypass $$http_x_hls_renew; # asked to replace a session that is gone
          proxy_ignore_headers Cache-Control Expires Set-Cookie Vary X-Accel-Expires;
          add_header X-Hls-Sess $$upstream_cache_status always;
          proxy_pass $$http_x_hls_provider;
        }
      }

      # ---- PROVIDERS: one server block per provider is built at start from
      # ---- .env (see "entrypoint" above) and shares provider-locations below.

  # Shared by every provider block (one per provider in .env).
  provider-locations:
    content: |
      access_log /dev/stdout masked;
      set $$sess   "-";
      set $$cdn    "";
      set $$chmode "";
      set $$pbase  "";

      # ---- The address the player asked for. Normally that is the request
      # ---- itself. A live HLS channel is handed its own folder, /__ch/<the
      # ---- channel's address>/ (see part 4), so there the channel's address is
      # ---- taken back out of the request.
      set $$orig $$request_uri;
      if ($$request_uri ~ "^(?<rp>[^?]*)(?<rq>\?.*)?$$") { set $$chmode ""; }
      if ($$request_uri ~ "^/__ch(?<co>/.*?\.m3u8)/(?<cn>[^?]*)(?<cq>\?.*)?$$") { set $$orig $$co; set $$chmode piece; }
      if ($$request_uri ~ "^/__ch(?<cop>/.*?\.m3u8)/__index\.m3u8(?<cqp>\?.*)?$$") { set $$orig "$$cop$$cqp"; set $$chmode playlist; }

      # ---- 5. One easy login for everything behind this proxy. A request that
      # ---- logs in as PLAYER_USER is sent on with the provider account of its
      # ---- port; any other login is passed on untouched (and refused by the provider).
      # ---- $$up_uri is the address sent to the provider: the player's own address,
      # ---- byte for byte, with only the login swapped. $$acct_user / $$acct_pass are
      # ---- already percent-encoded (see "entrypoint"), so any character is safe.
      set $$up_uri $$orig;
      # (named pieces, not $$1 $$2: nginx re-encodes numbered ones when the address contains %XX)
      if ($$orig ~ "^/+(?<pa>live|movie|series|timeshift)/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+(?<pb>.*)$$") { set $$up_uri "/$$pa/$$acct_user/$$acct_pass/$$pb"; }
      if ($$orig ~ "^/+${PLAYER_USER}/+${PLAYER_PASSWORD}/+(?<pc>[^/?]+(\?.*)?)$$") { set $$up_uri "/$$acct_user/$$acct_pass/$$pc"; }
      if ($$up_uri ~ "^(?<qa>[^?]*\?(.*&)?)username=${PLAYER_USER}(?<qb>(&.*)?)$$") { set $$up_uri "$${qa}username=$$acct_user$$qb"; }
      if ($$up_uri ~ "^(?<qc>[^?]*\?(.*&)?)password=${PLAYER_PASSWORD}(?<qd>(&.*)?)$$") { set $$up_uri "$${qc}password=$$acct_pass$$qd"; }

      location = /healthz { access_log off; return 200; }
      location / { proxy_pass $$provider$$up_uri; }

      # The guide and the API are text, often large. Let the provider compress
      # them if the app asks (as it would directly), and keep the framing that
      # lets the app notice a download that was cut short.
      location = /xmltv.php {
        proxy_read_timeout 300s;
        chunked_transfer_encoding on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding $$http_accept_encoding;
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_pass $$provider$$up_uri;
      }
      location = /player_api.php {
        proxy_read_timeout 300s;
        chunked_transfer_encoding on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding $$http_accept_encoding;
        proxy_set_header X-Hls-Provider "";
        proxy_set_header X-Hls-Renew "";
        proxy_pass $$provider$$up_uri;
      }

      # A provider's ready-made M3U playlist lists every stream at the
      # provider's own address with the real login, so a player using it would
      # skip the VPN. Refused on purpose: use the Xtream login instead.
      location = /get.php { return 403; }

      # (Order matters below: nginx tries these patterns from top to bottom, so the
      # two for a channel's own folder come before the general ".m3u8" one.)

      # ---- Live HLS, the channel's playlist (the player arrives here from "the start", further down). Fetch the remembered
      # ---- session's playlist. The player always asks the same address and always
      # ---- gets the same session.
      location ~ ^/__ch/.+\.m3u8/__index\.m3u8$$ {
        if ($$chmode != playlist) { return 404; }
        sub_filter_types *;
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls {
        sub_filter_types *;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_read_timeout 25s;               # long enough for a channel that is slow to start, short enough to replace one that hangs
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }        # "//server/path"
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }     # "/some/path" with no server name
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|\[|[^/]*[@%])") { return 403; }
        if ($$next ~* "^(https?)://([^/]+)") { set $$cdn "/__cdn/$$1/$$2"; }
        error_page 400 401 403 404 410 429 500 502 503 504 509 520 521 522 523 524 = @hls_renew;   # the remembered session is gone
        proxy_pass $$next;
      }
      location @hls_renew {                   # open the channel again, once, and remember the new session
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew 1;
        error_page 301 302 303 307 308 = @hls_renewed;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_renewed {                 # whatever this one answers goes to the player
        sub_filter_types *;
        proxy_read_timeout 25s;
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|\[|[^/]*[@%])") { return 403; }
        if ($$next ~* "^(https?)://([^/]+)") { set $$cdn "/__cdn/$$1/$$2"; }
        proxy_pass $$next;
      }

      # ---- Live HLS, the pieces: anything else asked inside a channel's folder is a
      # ---- piece (or a further playlist) that the channel's playlist named with
      # ---- no server and no folder. It lives next to that playlist, so fetch it
      # ---- from the folder the channel's session is in.
      location ~ ^/__ch/ {
        if ($$chmode != piece) { return 404; }
        limit_except GET HEAD { deny all; }
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls_piece;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_piece {
        set $$sess $$upstream_http_x_hls_sess;
        set $$next $$upstream_http_location;
        if ($$next ~ "^//") { set $$next "$$pscheme:$$next"; }
        if ($$next ~ "^/[^/]") { set $$next "$$provider$$next"; }
        if ($$next ~* "^https?://(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|169\.254\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|0\.|\[|[^/]*[@%])") { return 403; }
        set $$pdir "/";
        if ($$next ~* "^https?://[^/?]+(?<nd>/[^?]*/)") { set $$pdir $$nd; }
        if ($$next ~* "^(?<ns>https?)://(?<nh>[^/?]+)") { set $$cdn "/__cdn/$$ns/$$nh"; set $$pbase "$$ns://$$nh"; }
        if ($$pbase = "") { return 502; }
        proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
        proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$ns/$$1$$2;
        proxy_redirect ~^(/[^/].*)$$ $$cdn$$1;
        proxy_pass $$pbase$$pdir$$cn$$cq;
      }

      # ---- Live HLS, the start: the channel is opened. Ask the memory where this
      # ---- channel's session is. If the provider sends the channel to another
      # ---- server (most do), give the channel a folder of its own on this proxy
      # ---- and send the player there. A playlist may name its pieces with no
      # ---- folder at all ("chunk_123.ts"); the player then asks for them inside
      # ---- that same folder, and "the pieces" above knows which channel they belong to.
      # ---- If the provider answers with the playlist itself, it is passed on as is.
      location ~ \.m3u8$$ {
        sub_filter_types *;                   # a playlist is rewritten whatever type the provider labels it
        proxy_redirect off;
        proxy_intercept_errors on;
        recursive_error_pages on;
        proxy_set_header Connection "";
        proxy_set_header Accept-Encoding "";
        proxy_set_header X-Hls-Provider $$provider;
        proxy_set_header X-Hls-Renew "";
        error_page 301 302 303 307 308 = @hls_open;
        proxy_pass http://127.0.0.1:8195$$up_uri;
      }
      location @hls_open {
        set $$sess $$upstream_http_x_hls_sess;
        return 302 /__ch$$rp/__index.m3u8$$rq;
      }

      # Everything a provider pointed at: video pieces, MPEG-TS, movies, the guide.
      location ^~ /__cdn/ {
        set $$cdn $$cdn_prefix;
        set $$cdn_s $$cdn_scheme;
        if ($$cdn_forbidden) { return 403; }
        if ($$cdn_target = "") { return 404; }
        limit_except GET HEAD { deny all; }   # players only ever fetch
        proxy_redirect ~*^(https?)://([^/]+)(/.*)?$$ /__cdn/$$1/$$2$$3;
        proxy_redirect ~^//([^/]+)(/.*)?$$ /__cdn/$$cdn_s/$$1$$2;
        proxy_redirect ~^(/[^/].*)$$ $$cdn$$1;   # "go to /some/path" on the same server
        proxy_pass $$cdn_target;

        # A playlist fetched from the other server: rewritten whatever type it is labelled.
        location ~ \.m3u8$$ {
          sub_filter_types *;
          set $$cdn $$cdn_prefix;
          set $$cdn_s $$cdn_scheme;
          if ($$cdn_forbidden) { return 403; }
          if ($$cdn_target = "") { return 404; }
          limit_except GET HEAD { deny all; }
          proxy_pass $$cdn_target;
        }
      }
```

</details>

## Everyday use

**Add a provider.** Add its four lines to `.env`, using a name of your choice
in capitals and a free port inside `PROXY_PORTS`:

```
SPORTS_URL=https://provider.example
SPORTS_PORT=8096
SPORTS_USER=the-username-they-gave-you
SPORTS_PASSWORD='the-p@ssword-they-gave-you'
```

Then run `docker compose up -d`. Only the proxy restarts; the VPN stays up.

**Remove a provider.** Delete its four lines and run `docker compose up -d`.

**Logins with odd characters.** Provider logins can contain anything. You do
not get to choose them, so the proxy takes whatever you were given. One rule
for the `.env` file: if a username or password has anything besides letters
and digits, put it between single quotes. Without them, a `$` or `#` is read
as something else and part of the login is silently lost.

**More providers than ports.** Widen `PROXY_PORTS` (for example `8095-8099`)
and run `docker compose up -d`. This one restarts the VPN too.

**See what is happening.** `docker logs iptv-nginx` has one line per request.
Addresses are never logged, because they contain logins.

## How it works

Most of what the proxy does is pass bytes along. Five things go beyond that,
and each one exists to keep the behavior the same as a direct connection:

1. **Redirects stay inside the tunnel.** Providers answer most requests with
   "go to that other server". The proxy rewrites that answer so the player
   comes back to the proxy, which then fetches from the other server through
   the VPN. Without this the player would go to that server directly.
2. **Playlists are pointed back at the proxy.** An HLS playlist is a list of
   video pieces. The proxy rewrites the addresses in it for the same reason.
3. **One session per live channel.** Described [above](#why-this-exists). When
   the provider ends a session, the proxy opens a new one once and carries on.
4. **One easy login.** You choose a simple username and password for your
   players. The proxy swaps it for the provider's real login on the way out,
   so the real logins are typed in one place only.
5. **It refuses to be pointed at your own network.** A request cannot make the
   proxy fetch from the machine it runs on or from private addresses.

Your settings are read once, when the proxy starts. A short shell loop turns
each provider block in `.env` into its own listener on its own port.

## What was tested, and how

The goal is parity with direct credentials, and no gap is known. Some of that
was confirmed against real providers and some only against simulated ones, so
here is which is which.

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

**Confirmed against simulated providers only.** These are behaviors a provider
could have that the real ones used for testing did not, so a stand-in was
built for each. All of them pass, but none has been seen against a real
service:

- Playlists that name their pieces by bare file name (`chunk_123.ts`),
  including a further playlist in a sub-folder
- A provider that answers with the playlist itself instead of a redirect
- Redirects written in less common forms
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
- **Channel logos and posters do not go through the VPN.** The player fetches
  them straight from wherever they are hosted. The reason: a provider's
  channel and movie lists give each picture as a full web address, usually on
  some other site (a picture host, a movie database), not on the provider's
  own server. The proxy passes those lists on exactly as the provider sent
  them, so the player goes to those addresses itself. Sending them through
  the VPN would mean rewriting every list the provider sends, which is the
  opposite of behaving like a direct connection. This is the same with the
  provider's login typed straight into a player. They are only pictures; no
  video comes that way.
- **Your logins are in `.env` in plain text**, and the session memory
  (below) holds them too. Keep the folder private.
- **A slow provider is still slow.** If a provider's server falls behind, the
  proxy cannot make up the difference. See
  [When something is wrong](#when-something-is-wrong) for how to tell that
  apart from a proxy problem.
- **Changing a VPN setting interrupts playback** for a few seconds while the
  tunnel reconnects. Changing a provider does not touch the VPN.

## What it stores, updating, removing

**What it stores.** Besides the two files you put there, one small Docker
volume named `iptv-vpn_hls-memory`. Docker creates and manages it; there is no
folder for you to make or look after.

- For each live HLS channel being watched it holds one file of about 3 KB: the
  provider's "this channel is on that server" answer. The file includes the
  channel's address at the provider, **with your real login in it**.
- A channel's file is dropped one minute after the last request for it.
- It exists so that when the proxy container is recreated (you added a
  provider, or updated), channels that are playing carry on with the same
  session instead of being given a new one, which can show as buffering.

**Updating.**

- You changed `.env`: run `docker compose up -d`.
- You replaced `compose.yaml` with a newer version: run
  `docker compose up -d --force-recreate`. A plain `up -d` can leave the proxy
  running its old configuration.
- New versions of gluetun or nginx: `docker compose pull`, then
  `docker compose up -d`. The images are not pinned to a version on purpose: a
  VPN client and a proxy should get their security fixes.

**Removing.**

```
docker compose down -v
```

The `-v` matters: it removes the session volume too. A plain `down` leaves it
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

- **A provider does not answer.** Read the `iptv-nginx` log from its last
  start. It names each provider it found and each one it skipped, with the
  reason.
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
- **Reading the log.** `k=` is the kind of request. `s=` shows whether a
  channel's session was reused (`HIT`), opened (`MISS`) or replaced
  (`BYPASS`).

## Support

There is none. This was made for personal use and is shared as is, free, in
case it is useful to someone else. Issues and pull requests are turned off,
and questions will not be answered. Everything needed to use it is on this
page. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE). gluetun and nginx are separate projects with their own
licenses; this repository contains only configuration for them and does not
distribute either one.
