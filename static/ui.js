// After a full form POST the browser reloads at the top of the page. Remember
// where we were, jump back, and show a short "Saved!" toast. HTMX swaps skip
// this because they do not reload the page.
(function () {
    "use strict";

    var STORAGE_KEY = "school-nanny-scroll";

    try {
        if ("scrollRestoration" in history) {
            history.scrollRestoration = "manual";
        }
    } catch (e) {
        // Some embedded browsers refuse this; scrolling to the top is fine.
    }

    function skipToast(form) {
        if (form.classList.contains("danger-zone") || form.closest(".danger-zone")) {
            return true;
        }
        var action = (form.getAttribute("action") || "").toLowerCase();
        if (action.indexOf("/delete") !== -1) {
            return true;
        }
        if (action === "/login" || action.indexOf("/login") !== -1) {
            return true;
        }
        if (action === "/logout" || action.indexOf("/logout") !== -1) {
            return true;
        }
        return false;
    }

    function sentence(list) {
        if (list.length < 2) {
            return list.join("");
        }
        return list.slice(0, -1).join(", ") + " and " + list[list.length - 1];
    }

    // Saving a new date on a planned curriculum lesson is the same move as
    // dragging its card. When she pulls it earlier past work that is still
    // pending, the set closes up behind it — so ask before rearranging days
    // she is not looking at. Pushing later commits quietly, matching drag.
    function dateChangeWarning(form) {
        if (!form || !form.getAttribute("data-in-plan")) {
            return "";
        }
        var wasOn = form.getAttribute("data-was-on");
        var dateInput = form.querySelector('input[name="scheduled_on"]');
        var statusInput = form.querySelector('select[name="status"]');
        if (!dateInput || !wasOn) {
            return "";
        }
        var date = dateInput.value;
        if (!date || date >= wasOn) {
            return "";
        }
        if (statusInput && statusInput.value && statusInput.value !== "planned") {
            return "";
        }

        var sequence = Number(form.getAttribute("data-sequence"));
        var mates = form.querySelectorAll(".plan-mate");
        var sharing = [];
        var following = 0;
        var jumped = 0;
        var shareLabel = date;
        for (var i = 0; i < mates.length; i++) {
            var mate = mates[i];
            var on = mate.getAttribute("data-date");
            if (on < date) {
                continue;
            }
            if (Number(mate.getAttribute("data-sequence")) < sequence) {
                jumped++;
            }
            if (on === date) {
                sharing.push(mate.getAttribute("data-title"));
                shareLabel = mate.getAttribute("data-label") || date;
            } else {
                following++;
            }
        }
        if (!jumped) {
            return "";
        }

        var title = form.getAttribute("data-title") || "That lesson";
        var parts = [title + " belongs to a curriculum set."];
        if (sharing.length) {
            parts.push("It will share " + shareLabel + " with " +
                sentence(sharing) + ".");
        }
        if (following) {
            parts.push("The " + following + " lesson" + (following === 1 ? "" : "s") +
                " still ahead of it will close up behind, one school day each.");
        }
        parts.push("Save anyway?");
        return parts.join(" ");
    }

    document.addEventListener("submit", function (event) {
        var form = event.target;
        if (!form || form.tagName !== "FORM") {
            return;
        }
        if (form.hasAttribute("hx-post") || form.hasAttribute("hx-get")) {
            return;
        }

        if (form.classList.contains("lesson-save")) {
            var warning = dateChangeWarning(form);
            if (warning && !window.confirm(warning)) {
                event.preventDefault();
                return;
            }
        }

        var toast = "";
        if (!skipToast(form)) {
            toast = form.getAttribute("data-toast") || "Saved!";
        }
        try {
            sessionStorage.setItem(STORAGE_KEY, JSON.stringify({
                path: location.pathname,
                search: location.search,
                y: window.scrollY || window.pageYOffset || 0,
                toast: toast
            }));
        } catch (e) {
            // Private windows can block sessionStorage; the save still works.
        }
    });

    function showToast(text) {
        if (!text) {
            return;
        }
        var el = document.createElement("div");
        el.className = "save-toast";
        el.setAttribute("role", "status");
        el.setAttribute("aria-live", "polite");
        el.textContent = text;
        document.body.appendChild(el);
        window.setTimeout(function () {
            el.classList.add("is-leaving");
            window.setTimeout(function () {
                if (el.parentNode) {
                    el.parentNode.removeChild(el);
                }
            }, 400);
        }, 1500);
    }

    var stashed = null;
    try {
        var raw = sessionStorage.getItem(STORAGE_KEY);
        if (raw) {
            stashed = JSON.parse(raw);
            sessionStorage.removeItem(STORAGE_KEY);
        }
    } catch (e) {
        stashed = null;
    }

    if (!stashed) {
        return;
    }
    if (stashed.path === location.pathname) {
        var y = Number(stashed.y) || 0;
        window.scrollTo(0, y);
    }
    if (stashed.toast) {
        showToast(stashed.toast);
    }
})();
