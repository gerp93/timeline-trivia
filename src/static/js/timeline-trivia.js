// TimelineTrivia Game JavaScript

let timelineTriviaConn = null;

// Per-turn countdown, in seconds; 0 = off. Kept in sync with the lobby setting
// via the "turnTimer:" websocket hint so a change takes effect without a
// reload.
let timelineTriviaTurnTimerSeconds = 0;

// setTurnTimerSeconds updates the tracked value and shows/hides the "Time:"
// stat to match. #turn-timer-stat is always rendered (even when the lobby
// started with the timer off) specifically so this can show it later — a
// timer turned on mid-game needs the element to already exist for
// doRestartTurnTimer to find, not just the seconds value to be nonzero.
function setTurnTimerSeconds(seconds) {
    timelineTriviaTurnTimerSeconds = parseInt(seconds) || 0;
    const statEl = document.getElementById("turn-timer-stat");
    if (statEl) {
        statEl.style.display = timelineTriviaTurnTimerSeconds > 0 ? "" : "none";
    }
}

// How long the bottom status line stays up. Every client writes every guess
// outcome here, so it needs to last long enough to read but clear before it
// goes stale.
const STATUS_MESSAGE_MS = 8000;
let statusMessageTimeout = null;

// True from the moment a "result:" (correct/incorrect/revealed) popup is
// requested until that popup — and, if the timer is on, the turn-countdown
// popup after it — has finished. While true, restartTurnTimer defers: the
// clock must not start (or keep ticking) behind a full-screen popup that
// covers the board.
let deferTimerStart = false;

// The showResultPopup/showTurnCountdown/showTimerChangeAnnouncement trio all
// share one backdrop on screen at a time, and each used to just yank the
// previous popup's DOM node out from under it (`existing.remove()`) when a
// new one started. That left the outgoing popup's own setTimeout/setInterval
// running invisibly in the background — no longer visible, but still due to
// fire its onDone later. A stale onDone could flip deferTimerStart back to
// false (or replay a 3-2-1 countdown) out of step with whatever sequence was
// actually on screen, cutting the real per-turn timer short a few seconds
// after it had just (re)started. dismissActivePopup lets each show* function
// properly cancel its predecessor — clearing its timer and skipping its
// onDone — instead of leaving it to fire late.
let dismissActivePopup = null;

function dismissAnyActivePopup() {
    if (dismissActivePopup) {
        const dismiss = dismissActivePopup;
        dismissActivePopup = null;
        // false = don't fire the outgoing popup's onDone; the caller is
        // about to start its own sequence and owns what happens next.
        dismiss(false);
    }
}

function initTimelineTriviaWebSocket(lobbyId, playerId, turnTimerSeconds) {
    setTurnTimerSeconds(turnTimerSeconds);

    let wsProtocol = "wss://";
    if (document.location.protocol === "http:") {
        wsProtocol = "ws://";
    }

    timelineTriviaConn = new WebSocket(wsProtocol + document.location.host + "/ws/lobby/" + lobbyId);

    if (!timelineTriviaConn) {
        alert("Failed to make connection.");
        document.location.href = "/timeline-trivia/lobbies";
        return;
    }

    timelineTriviaConn.onclose = () => {
        alert("Connection Lost");
        document.location.href = "/timeline-trivia/lobbies";
    };

    // Chat form handling
    const chatForm = document.getElementById("timeline-trivia-chat-form");
    const chatMessages = document.getElementById("timeline-trivia-chat-messages");
    const chatInput = document.getElementById("timeline-trivia-chat-input");

    gsChat.wireForm(chatForm, chatInput, timelineTriviaConn);

    // The timeline fragment is what says whose turn it is, and on first paint
    // it arrives via its own hx-trigger="load" rather than through
    // refreshGameState — so start the clock off that swap.
    document.body.addEventListener("htmx:afterSwap", (event) => {
        if (event.detail.target && event.detail.target.id === "timeline-trivia-timeline") {
            restartTurnTimer(lobbyId);
        }
    });

    timelineTriviaConn.onmessage = (event) => {
        let messageText = event.data;
        console.log("[TimelineTrivia WS] Received:", messageText);

        switch (messageText) {
            case "refresh":
                // Refresh all game components
                console.log("[TimelineTrivia WS] Refreshing game state...");
                refreshGameState(lobbyId);
                return;

            case "reload":
                // Game start/reset: refresh game state and controls without a
                // full page navigation. A full location.reload() drops this
                // websocket connection; if this player is the only client,
                // the server deletes the (now empty) lobby before the reload
                // can finish, destroying the game that was just started.
                console.log("[TimelineTrivia WS] Refreshing game in 500ms...");
                setTimeout(() => {
                    refreshGameState(lobbyId);
                    refreshControls(lobbyId);
                }, 500);
                return;

            case "kick":
                document.location.href = "/timeline-trivia/lobbies";
                return;
        }

        // Handle result popups (correct/incorrect placement). The payload is
        // JSON so player names and card text can contain anything, and so the
        // winner's celebration can ride along.
        if (messageText.startsWith("result:")) {
            let payload;
            try {
                payload = JSON.parse(messageText.substring("result:".length));
            } catch (e) {
                console.error("[TimelineTrivia] bad result payload:", e);
                return;
            }
            // Everyone sees what happened, not just the player who acted.
            showStatusMessage(payload.bottomMessage);

            // Freeze the visible clock immediately — it belongs to a turn
            // that just ended, and must not keep ticking (or hit zero)
            // behind the result popup.
            deferTimerStart = true;
            if (typeof gsTimer !== "undefined") gsTimer.stop();

            showResultPopup(payload, () => {
                const next = payload.nextPlayerName;
                if (next && timelineTriviaTurnTimerSeconds > 0) {
                    // Only once this second popup clears does the board
                    // "reopen" and the real timer start.
                    showTurnCountdown(next + "'s turn", () => {
                        deferTimerStart = false;
                        doRestartTurnTimer(lobbyId);
                    });
                } else {
                    deferTimerStart = false;
                    doRestartTurnTimer(lobbyId);
                }
            });
            return;
        }

        // Lightweight status-line-only update, for events that aren't a
        // guess outcome (e.g. Skip & Remove) and so don't get a popup.
        if (messageText.startsWith("status:")) {
            showStatusMessage(messageText.substring("status:".length));
            return;
        }

        // Handle chat messages
        if (messageText.startsWith("chat:")) {
            const chatContent = messageText.substring(5);
            addChatMessage(chatContent);
            return;
        }

        // Handle alert messages
        if (messageText.startsWith("alert:")) {
            const alertContent = messageText.substring(6);
            showAlert(alertContent);
            return;
        }

        // Handle lobby message updates (shown persistently under the lobby name)
        if (messageText.startsWith("lobbyMessage:")) {
            const lobbyMessage = messageText.substring("lobbyMessage:".length);
            updateLobbyMessageBanner(lobbyMessage);
            return;
        }

        // Handle turn timer setting changes (framework lobby setting). Everyone
        // sees the same announcement + countdown before the clock restarts,
        // same "no one sees the board change out from under them" rule the
        // per-turn countdown already follows.
        if (messageText.startsWith("turnTimer:")) {
            const newSeconds = parseInt(messageText.substring("turnTimer:".length)) || 0;
            setTurnTimerSeconds(newSeconds);

            // Nothing to announce or restart if there's no turn in progress
            // yet (game hasn't started, or just ended) — the value is still
            // tracked for whenever a turn does start.
            const hasCurrentGuesser = !!document.querySelector(".player-timeline-row.is-current");
            if (!hasCurrentGuesser) return;

            const message = newSeconds > 0
                ? "Turn timer set to " + newSeconds + "s"
                : "Turn timer turned off";

            deferTimerStart = true;
            if (typeof gsTimer !== "undefined") gsTimer.stop();

            showTimerChangeAnnouncement(message, () => {
                if (newSeconds > 0) {
                    showTurnCountdown("Timer starting...", () => {
                        deferTimerStart = false;
                        doRestartTurnTimer(lobbyId);
                    });
                } else {
                    deferTimerStart = false;
                    doRestartTurnTimer(lobbyId);
                }
            });
            return;
        }

        // Default: treat as chat message
        addChatMessage(messageText);
    };
}

function refreshGameState(lobbyId) {
    console.log("[TimelineTrivia] refreshGameState called with lobbyId:", lobbyId);

    // Refresh current card
    htmx.ajax("GET", "/api/timeline-trivia/" + lobbyId + "/current-card", {
        target: "#current-card-content",
        swap: "innerHTML"
    }).then(() => console.log("[TimelineTrivia] current-card refreshed"));

    // Refresh timeline using fetch directly
    const timelineTarget = document.getElementById("timeline-trivia-timeline");
    console.log("[TimelineTrivia] Timeline target element:", timelineTarget);
    const timelineUrl = "/api/timeline-trivia/" + lobbyId + "/timeline?t=" + Date.now();
    console.log("[TimelineTrivia] Fetching timeline from:", timelineUrl);
    fetch(timelineUrl, { cache: 'no-store' })
        .then(response => response.text())
        .then(html => {
            console.log("[TimelineTrivia] Got timeline HTML, length:", html.length);
            if (timelineTarget) {
                timelineTarget.innerHTML = html;
                htmx.process(timelineTarget); // Process HTMX attributes in new content
            }
            console.log("[TimelineTrivia] timeline refreshed");
            // The timeline is what says whose turn it is, so the countdown can
            // only be (re)started once it has landed.
            restartTurnTimer(lobbyId);
        })
        .catch(e => console.error("[TimelineTrivia] timeline error:", e));

    // Refresh the deck breakdown shown in the header tooltip
    htmx.ajax("GET", "/api/timeline-trivia/" + lobbyId + "/decks", {
        target: "#deck-info-tooltip",
        swap: "innerHTML"
    }).catch(e => console.error("[TimelineTrivia] decks error:", e));

    // Refresh draw pile count
    fetch("/api/timeline-trivia/" + lobbyId + "/draw-pile-count", { cache: 'no-store' })
        .then(response => response.text())
        .then(count => {
            const el = document.getElementById("draw-pile-count");
            if (el) {
                el.innerHTML = "Remaining: <strong>" + count + "</strong>";
            }
        })
        .catch(e => console.error("[TimelineTrivia] draw-pile-count error:", e));
}

function refreshControls(lobbyId) {
    // Re-fetches the current page and swaps in just the #timeline-trivia-controls
    // block (Start/Reset button, waiting/winner text) so a game-status change
    // is reflected without a full page navigation.
    fetch(location.pathname, { cache: "no-store" })
        .then(response => response.text())
        .then(html => {
            const doc = new DOMParser().parseFromString(html, "text/html");
            const newControls = doc.getElementById("timeline-trivia-controls");
            const currentControls = document.getElementById("timeline-trivia-controls");
            if (newControls && currentControls) {
                currentControls.outerHTML = newControls.outerHTML;
                htmx.process(document.getElementById("timeline-trivia-controls"));
            }
        })
        .catch(e => console.error("[TimelineTrivia] controls refresh error:", e));
}

// restartTurnTimer is the guarded entry point every refresh path calls. It
// defers to doRestartTurnTimer immediately UNLESS a result/countdown popup
// sequence is in flight, in which case that sequence's own completion
// callback is what eventually starts the clock (see the "result:" handler).
function restartTurnTimer(lobbyId) {
    if (deferTimerStart) return;
    doRestartTurnTimer(lobbyId);
}

// doRestartTurnTimer runs the countdown afresh for the current guesser's
// turn, unconditionally — callers are responsible for the "not while a popup
// is showing" rule above. Everyone watches the same clock, but only the
// guesser's own browser reports the timeout — the server re-checks whose
// turn it is regardless.
//
// The seconds it counts down from come from the server (the
// #timeline-container fragment's data-turn-seconds-remaining, computed
// against TIMELINE_TRIVIA_GAME.CURRENT_TURN_STARTED_ON_DATE), not from
// timelineTriviaTurnTimerSeconds directly. Every client used to start its
// own fresh countdown from the full configured duration the moment ITS OWN
// result/celebration popup happened to clear — since players can click
// through those popups at very different speeds, whoever dismissed fastest
// got a head start on the same turn's clock, and the round could end while
// slower/spectating clients were still mid-animation. Reading the
// server-computed remaining time instead means every client — however fast
// or slow it got here — converges on the same true deadline.
function doRestartTurnTimer(lobbyId) {
    const timerEl = document.getElementById("turn-timer");
    if (!timerEl || typeof gsTimer === "undefined") return;

    if (!timelineTriviaTurnTimerSeconds) {
        gsTimer.reset(timerEl, 0);
        return;
    }

    // The timeline fragment marks the row that is both the current guesser's
    // and mine; its presence is how this client knows the clock is on it.
    const isMyTurn = !!document.querySelector(".player-timeline-row.is-current.is-me");

    // No current guesser (game waiting or finished) means no clock to run.
    const hasCurrentGuesser = !!document.querySelector(".player-timeline-row.is-current");
    if (!hasCurrentGuesser) {
        gsTimer.reset(timerEl, 0);
        return;
    }

    const container = document.getElementById("timeline-container");
    const serverRemaining = container ? parseInt(container.dataset.turnSecondsRemaining, 10) : NaN;
    const secondsRemaining = Number.isFinite(serverRemaining) ? serverRemaining : timelineTriviaTurnTimerSeconds;

    gsTimer.start(timerEl, secondsRemaining, () => {
        if (!isMyTurn) return;
        fetch("/api/timeline-trivia/" + lobbyId + "/timeout", { method: "POST" })
            .catch(e => console.error("[TimelineTrivia] timeout error:", e));
    });
}

// showTurnCountdown plays a 3-2-1 countdown under the given label, then
// calls onDone. Shown between the result popup clearing and the real timer
// starting, so nobody sees the board "reopen" until the clock is genuinely
// about to run. Only called when a turn timer is configured — see the
// "result:" handler, which skips straight to onDone when it's off.
function showTurnCountdown(label, onDone) {
    dismissAnyActivePopup();

    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup turn-countdown";

    const numberEl = document.createElement("div");
    numberEl.className = "countdown-number";
    popup.appendChild(numberEl);

    const nameEl = document.createElement("div");
    nameEl.className = "countdown-name";
    nameEl.textContent = label;
    popup.appendChild(nameEl);

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    let finished = false;
    let count = 3;
    numberEl.textContent = count;

    function finish(callOnDone) {
        if (finished) return;
        finished = true;
        clearInterval(interval);
        backdrop.remove();
        if (dismissActivePopup === finish) dismissActivePopup = null;
        if (callOnDone !== false && typeof onDone === "function") onDone();
    }
    dismissActivePopup = finish;

    const interval = setInterval(() => {
        count -= 1;
        if (count > 0) {
            numberEl.textContent = count;
        } else {
            finish();
        }
    }, 1000);

    // Click to skip the countdown, same as the result popup allows.
    backdrop.addEventListener("click", () => finish());
}

// showTimerChangeAnnouncement shows a brief message-only popup (no number),
// then calls onDone. Used before showTurnCountdown when the turn timer
// setting itself changes mid-game — see the "turnTimer:" handler — so
// everyone understands why the clock is about to reset before it does.
function showTimerChangeAnnouncement(message, onDone) {
    dismissAnyActivePopup();

    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup turn-countdown";

    const messageEl = document.createElement("div");
    messageEl.className = "countdown-announcement";
    messageEl.textContent = message;
    popup.appendChild(messageEl);

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    let finished = false;
    function finish(callOnDone) {
        if (finished) return;
        finished = true;
        clearTimeout(dismissTimer);
        backdrop.remove();
        if (dismissActivePopup === finish) dismissActivePopup = null;
        if (callOnDone !== false && typeof onDone === "function") onDone();
    }
    dismissActivePopup = finish;

    const dismissTimer = setTimeout(finish, 1400);
    backdrop.addEventListener("click", () => finish());
}

function addChatMessage(message) {
    // Shared renderer: parses <blue>/<green>/<red>/</> color tokens, timestamps,
    // and trims history (see gameshell-framework /gs/js/chat.js).
    gsChat.append(document.getElementById("timeline-trivia-chat-messages"), message);
}

function updateLobbyMessageBanner(message) {
    const banner = document.getElementById("lobby-message-banner");
    if (!banner) return;
    banner.textContent = message;
    banner.style.display = message ? "" : "none";
}

// showStatusMessage writes the bottom-of-screen status line. Driven by the
// websocket rather than by the acting player's own HTTP response, so every
// player sees every outcome.
function showStatusMessage(message) {
    if (!message) return;
    const messageDiv = document.getElementById("timeline-trivia-message");
    if (!messageDiv) return;

    messageDiv.textContent = message;
    if (statusMessageTimeout) clearTimeout(statusMessageTimeout);
    statusMessageTimeout = setTimeout(() => {
        messageDiv.textContent = "";
    }, STATUS_MESSAGE_MS);
}

function showAlert(message) {
    showStatusMessage(message);
}

// showResultPopup renders the guess-outcome modal. On a correct guess it also
// shows the winner's personal celebration GIF/message if they set one on their
// account page. onDone fires exactly once, whether the popup auto-dismisses
// or is clicked away.
function showResultPopup(payload, onDone) {
    // Cancel any existing popup — clears its pending timer too, not just its
    // DOM node, so its onDone can't fire late and out of step (see
    // dismissActivePopup above).
    dismissAnyActivePopup();

    // Create backdrop
    const backdrop = document.createElement("div");
    backdrop.className = "timeline-trivia-popup-backdrop";

    // "revealed" = every player missed the card; it reuses the "incorrect"
    // styling but reveals the actual year and stays up longer since there's
    // more to read.
    const isRevealed = payload.type === "revealed";
    const styleClass = isRevealed ? "incorrect" : payload.type;

    // Create popup
    const popup = document.createElement("div");
    popup.className = "timeline-trivia-popup " + styleClass;

    const icon = payload.type === "correct" ? "✓" : "✗";
    const title = payload.type === "correct" ? "CORRECT!" : isRevealed ? "NOBODY GOT IT!" : "WRONG!";

    // Built node-by-node rather than with innerHTML: player names and card
    // text are user-authored and must not be parsed as markup.
    const iconEl = document.createElement("span");
    iconEl.className = "popup-icon";
    iconEl.textContent = icon;
    popup.appendChild(iconEl);
    popup.appendChild(document.createTextNode(title));

    const messageEl = document.createElement("div");
    messageEl.className = "popup-message";
    messageEl.textContent = payload.playerName + ": " + payload.message;
    popup.appendChild(messageEl);

    // "incorrect" carries a lose-celebration the same way "correct" carries a
    // win-celebration; "revealed" never does (no single player to show it to).
    const isCelebratable = payload.type === "correct" || payload.type === "incorrect";
    const hasCelebration = isCelebratable && (payload.hasGif || payload.celebration);
    if (payload.hasGif && payload.userId) {
        const gif = document.createElement("img");
        gif.className = "popup-gif";
        gif.alt = "";
        const gifRoute = payload.type === "correct" ? "win-gif" : "lose-gif";
        gif.src = "/api/user/" + encodeURIComponent(payload.userId) + "/" + gifRoute;
        popup.appendChild(gif);
    }
    if (isCelebratable && payload.celebration) {
        const celebrationEl = document.createElement("div");
        celebrationEl.className = "popup-celebration-message";
        celebrationEl.textContent = payload.celebration;
        popup.appendChild(celebrationEl);
    }

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    let finished = false;
    function finish(callOnDone) {
        if (finished) return;
        finished = true;
        clearTimeout(dismissTimer);
        backdrop.remove();
        if (dismissActivePopup === finish) dismissActivePopup = null;
        if (callOnDone !== false && typeof onDone === "function") onDone();
    }
    dismissActivePopup = finish;

    // Auto-remove; "revealed" and celebrations get extra time to read
    let dismissAfter = 2000;
    if (isRevealed) dismissAfter = 5000;
    if (hasCelebration) dismissAfter = 4000;
    const dismissTimer = setTimeout(finish, dismissAfter);

    // Also allow click to dismiss
    backdrop.addEventListener("click", () => finish());
}
