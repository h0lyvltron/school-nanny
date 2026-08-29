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

    document.addEventListener("submit", function (event) {
        var form = event.target;
        if (!form || form.tagName !== "FORM") {
            return;
        }
        if (form.hasAttribute("hx-post") || form.hasAttribute("hx-get")) {
            return;
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
