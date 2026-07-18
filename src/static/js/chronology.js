// Chronology Game JavaScript

let chronologyConn = null;

function initChronologyWebSocket(lobbyId, playerId) {
    let wsProtocol = "wss://";
    if (document.location.protocol === "http:") {
        wsProtocol = "ws://";
    }

    chronologyConn = new WebSocket(wsProtocol + document.location.host + "/ws/lobby/" + lobbyId);

    if (!chronologyConn) {
        alert("Failed to make connection.");
        document.location.href = "/chronology/lobbies";
        return;
    }

    chronologyConn.onclose = () => {
        alert("Connection Lost");
        document.location.href = "/chronology/lobbies";
    };

    // Chat form handling
    const chatForm = document.getElementById("chronology-chat-form");
    const chatMessages = document.getElementById("chronology-chat-messages");
    const chatInput = document.getElementById("chronology-chat-input");

    if (chatForm) {
        chatForm.onsubmit = (event) => {
            event.preventDefault();
            if (!chatInput.value) return;
            chronologyConn.send(chatInput.value);
            chatInput.value = "";
        };
    }

    chronologyConn.onmessage = (event) => {
        let messageText = event.data;
        console.log("[Chronology WS] Received:", messageText);

        switch (messageText) {
            case "refresh":
                // Refresh all game components
                console.log("[Chronology WS] Refreshing game state...");
                refreshGameState(lobbyId);
                return;

            case "reload":
                // Game start/reset: refresh game state and controls without a
                // full page navigation. A full location.reload() drops this
                // websocket connection; if this player is the only client,
                // the server deletes the (now empty) lobby before the reload
                // can finish, destroying the game that was just started.
                console.log("[Chronology WS] Refreshing game in 500ms...");
                setTimeout(() => {
                    refreshGameState(lobbyId);
                    refreshControls(lobbyId);
                }, 500);
                return;

            case "kick":
                document.location.href = "/chronology/lobbies";
                return;
        }

        // Handle result popups (correct/incorrect placement)
        if (messageText.startsWith("result:")) {
            const parts = messageText.split(":");
            // format: result:playerName:correct/incorrect:message
            const playerName = parts[1];
            const resultType = parts[2]; // "correct" or "incorrect"
            const message = parts.slice(3).join(":");
            showResultPopup(playerName, resultType, message);
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

        // Default: treat as chat message
        addChatMessage(messageText);
    };
}

function refreshGameState(lobbyId) {
    console.log("[Chronology] refreshGameState called with lobbyId:", lobbyId);
    
    // Refresh current card
    htmx.ajax("GET", "/api/chronology/" + lobbyId + "/current-card", {
        target: "#current-card-content",
        swap: "innerHTML"
    }).then(() => console.log("[Chronology] current-card refreshed"));

    // Refresh timeline using fetch directly
    const timelineTarget = document.getElementById("chronology-timeline");
    console.log("[Chronology] Timeline target element:", timelineTarget);
    const timelineUrl = "/api/chronology/" + lobbyId + "/timeline?t=" + Date.now();
    console.log("[Chronology] Fetching timeline from:", timelineUrl);
    fetch(timelineUrl, { cache: 'no-store' })
        .then(response => response.text())
        .then(html => {
            console.log("[Chronology] Got timeline HTML, length:", html.length);
            if (timelineTarget) {
                timelineTarget.innerHTML = html;
                htmx.process(timelineTarget); // Process HTMX attributes in new content
            }
            console.log("[Chronology] timeline refreshed");
        })
        .catch(e => console.error("[Chronology] timeline error:", e));

    // Refresh players list
    htmx.ajax("GET", "/api/chronology/" + lobbyId + "/players", {
        target: "#players-inline",
        swap: "innerHTML"
    }).then(() => console.log("[Chronology] players refreshed"));

    // Refresh draw pile count
    fetch("/api/chronology/" + lobbyId + "/draw-pile-count", { cache: 'no-store' })
        .then(response => response.text())
        .then(count => {
            const el = document.getElementById("draw-pile-count");
            if (el) {
                el.innerHTML = "Remaining: <strong>" + count + "</strong>";
            }
        })
        .catch(e => console.error("[Chronology] draw-pile-count error:", e));
}

function refreshControls(lobbyId) {
    // Re-fetches the current page and swaps in just the #chronology-controls
    // block (Start/Reset button, waiting/winner text) so a game-status change
    // is reflected without a full page navigation.
    fetch(location.pathname, { cache: "no-store" })
        .then(response => response.text())
        .then(html => {
            const doc = new DOMParser().parseFromString(html, "text/html");
            const newControls = doc.getElementById("chronology-controls");
            const currentControls = document.getElementById("chronology-controls");
            if (newControls && currentControls) {
                currentControls.outerHTML = newControls.outerHTML;
                htmx.process(document.getElementById("chronology-controls"));
            }
        })
        .catch(e => console.error("[Chronology] controls refresh error:", e));
}

function addChatMessage(message) {
    const chatMessages = document.getElementById("chronology-chat-messages");
    if (!chatMessages) return;

    const messageDiv = document.createElement("div");
    messageDiv.className = "chat-message";
    messageDiv.textContent = message;
    chatMessages.appendChild(messageDiv);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

function showAlert(message) {
    // Could use a modal or toast notification
    const messageDiv = document.getElementById("chronology-message");
    if (messageDiv) {
        messageDiv.textContent = message;
        messageDiv.className = "alert-message";
        setTimeout(() => {
            messageDiv.textContent = "";
        }, 3000);
    }
}

function showResultPopup(playerName, resultType, message) {
    // Remove any existing popup
    const existing = document.querySelector(".chronology-popup-backdrop");
    if (existing) existing.remove();

    // Create backdrop
    const backdrop = document.createElement("div");
    backdrop.className = "chronology-popup-backdrop";

    // Create popup
    const popup = document.createElement("div");
    popup.className = "chronology-popup " + resultType;

    const icon = resultType === "correct" ? "✓" : "✗";
    const title = resultType === "correct" ? "CORRECT!" : "WRONG!";

    popup.innerHTML = `
        <span class="popup-icon">${icon}</span>
        ${title}
        <div class="popup-message">${playerName}: ${message}</div>
    `;

    backdrop.appendChild(popup);
    document.body.appendChild(backdrop);

    // Auto-remove after 2 seconds
    setTimeout(() => {
        backdrop.remove();
    }, 2000);

    // Also allow click to dismiss
    backdrop.addEventListener("click", () => {
        backdrop.remove();
    });
}

// HTMX event handlers for after-swap updates
document.addEventListener("htmx:afterSwap", function(event) {
    // Handle any post-swap logic if needed
});
