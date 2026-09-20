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

// Every button-like control should explain its result before it is clicked.
// Explicit title or data-hint text wins; these defaults also cover controls
// inserted later by HTMX.
(function () {
    "use strict";

    var exactHints = {
        "push": "Move this lesson and every later planned lesson to the next available school days.",
        "push this and later": "Move this lesson and every later planned lesson to the next available school days.",
        "pull": "Move this future lesson onto today and pull every later planned lesson forward behind it.",
        "pull this and later": "Move this future lesson onto today and pull every later planned lesson forward behind it.",
        "double up": "Put this lesson on the same day as the previous lesson and pull later lessons forward.",
        "shift": "Move this standalone lesson one school day later.",
        "shift remaining": "Choose a new date for the remaining planned lessons while preserving their sequence.",
        "pause and shift remaining": "Pause this curriculum schedule, then move all unfinished lessons to resume on a chosen date.",
        "shift around vacation": "Move unfinished lessons around a vacation date range without changing their order.",
        "mark done": "Mark this lesson complete and include it in completed-work progress.",
        "mark not done": "Return this completed lesson to its previous unfinished status.",
        "undo": "Restore the lesson that was just deleted.",
        "undo deletion": "Restore this deleted lesson, including its saved files and assessment links.",
        "restore": "Restore this deleted lesson, including its saved files and assessment links.",
        "print": "Open the browser print dialog for this page.",
        "print today": "Open the browser print dialog for today's schedule.",
        "print week": "Open the browser print dialog for this week's planner.",
        "previous": "Move to the previous date range.",
        "next": "Move to the next date range.",
        "this week": "Return the planner to the current week.",
        "this month": "Return the calendar to the current month.",
        "zoom in": "Make the displayed PDF page larger.",
        "zoom out": "Make the displayed PDF page smaller.",
        "fit width": "Scale the PDF page to fit the available viewer width.",
        "fit page": "Scale the full PDF page to fit inside the viewer.",
        "reset": "Return the PDF viewer to its default zoom and position.",
        "move up": "Move this item one position earlier in its ordered list.",
        "move down": "Move this item one position later in its ordered list.",
        "sign out": "End this signed-in session on this device.",
        "lock": "Lock School Nanny until the family password or PIN is entered.",
        "history": "Review, undo, redo, and branch planner changes.",
        "return to planner": "Leave this page and return to the week planner."
    };

    function normalizedLabel(control) {
        return (control.getAttribute("aria-label") || control.textContent || "")
            .replace(/\s+/g, " ").trim();
    }

    function hintFor(control) {
        var explicit = control.getAttribute("data-hint");
        if (explicit) {
            return explicit;
        }
        if (control.classList.contains("emoji-pick")) {
            return "Choose " + (control.getAttribute("data-emoji") || control.textContent.trim()) +
                " as the displayed emoji.";
        }
        var label = normalizedLabel(control);
        var key = label.toLowerCase().replace(/^[←↑↓]\s*|\s*[→]$/g, "");
        if (exactHints[key]) {
            return exactHints[key];
        }
        if (/^(save|record)/.test(key)) {
            return "Save the information entered in this form.";
        }
        if (/^(add|create)/.test(key)) {
            return "Create and save the new item described in this form.";
        }
        if (/^(delete|remove|revoke|stop)/.test(key)) {
            return "Remove the selected item after any required confirmation.";
        }
        if (/^(import|attach|add photo|change photo)/.test(key)) {
            return "Upload the selected file and attach its information here.";
        }
        if (/^(open|show|full schedule|tests|year report|plan the week)/.test(key)) {
            return "Open " + label + ".";
        }
        if (control.tagName === "A") {
            return label ? "Open the " + label + " page." : "Open this page.";
        }
        return label ? "Perform the “" + label + "” action." : "Perform this action.";
    }

    function addButtonHints(root) {
        var controls = root.querySelectorAll ?
            root.querySelectorAll('button, a[role="button"], input[type="submit"], input[type="button"]') : [];
        for (var i = 0; i < controls.length; i++) {
            if (!controls[i].hasAttribute("title")) {
                controls[i].setAttribute("title", hintFor(controls[i]));
            }
        }
    }

    addButtonHints(document);
    document.addEventListener("htmx:afterSwap", function (event) {
        addButtonHints(event.target);
    });
})();

// Kids and Adults fly-outs in the header, and the compact person filter on
// Today/Week, close when she clicks away or hits Escape.
(function () {
    "use strict";

    function flyOutRoot(node) {
        if (!node || !node.closest) {
            return null;
        }
        return node.closest(".app-nav details.nav-people, details.person-filter-menu");
    }

    function closeFlyOuts(except) {
        var open = document.querySelectorAll(
            ".app-nav details.nav-people[open], details.person-filter-menu[open]"
        );
        for (var i = 0; i < open.length; i++) {
            if (open[i] !== except) {
                open[i].removeAttribute("open");
            }
        }
    }

    function flipList(list) {
        if (!list) {
            return;
        }
        list.style.left = "0";
        list.style.right = "auto";
        var box = list.getBoundingClientRect();
        if (box.right > window.innerWidth - 8) {
            list.style.left = "auto";
            list.style.right = "0";
        }
        box = list.getBoundingClientRect();
        if (box.left < 8) {
            list.style.left = "0";
            list.style.right = "auto";
        }
    }

    document.addEventListener("click", function (event) {
        closeFlyOuts(flyOutRoot(event.target));
    });

    document.addEventListener("keydown", function (event) {
        if (event.key === "Escape") {
            closeFlyOuts(null);
        }
    });

    document.addEventListener("toggle", function (event) {
        var details = event.target;
        if (!details || !details.classList) {
            return;
        }
        var list = details.querySelector(".nav-people-list, .person-filter-list");
        if (!list) {
            return;
        }
        list.style.left = "";
        list.style.right = "";
        if (!details.open) {
            return;
        }
        flipList(list);
    }, true);
})();

// Person chips on Today/Week/Attendance collapse to a fly-out when they would wrap.
(function () {
    "use strict";

    function layoutOne(box) {
        if (!box) {
            return;
        }
        box.classList.remove("is-compact");
        var chips = box.querySelector(".kid-filter");
        if (!chips) {
            return;
        }
        var overflow = chips.scrollWidth > chips.clientWidth + 1;
        box.classList.toggle("is-compact", overflow);
        if (!overflow) {
            var menu = box.querySelector(".person-filter-menu");
            if (menu) {
                menu.removeAttribute("open");
            }
        }
    }

    function eachBox(root, fn) {
        if (root && root.matches && root.matches("[data-person-filter]")) {
            fn(root);
        }
        var found = root && root.querySelectorAll ?
            root.querySelectorAll("[data-person-filter]") : [];
        for (var i = 0; i < found.length; i++) {
            fn(found[i]);
        }
    }

    var watching = typeof WeakSet === "function" ? new WeakSet() : null;
    var observer = typeof ResizeObserver === "function" ? new ResizeObserver(function (entries) {
        for (var i = 0; i < entries.length; i++) {
            layoutOne(entries[i].target);
        }
    }) : null;

    function watch(root) {
        eachBox(root || document, function (box) {
            if (observer && (!watching || !watching.has(box))) {
                if (watching) {
                    watching.add(box);
                }
                observer.observe(box);
            }
            layoutOne(box);
        });
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", function () {
            watch(document);
        });
    } else {
        watch(document);
    }
    document.addEventListener("htmx:afterSwap", function (event) {
        watch(event.target);
    });
    window.addEventListener("resize", function () {
        eachBox(document, layoutOne);
    });
})();

// After Double up / Shift / Push / Pull / Drop, keep keyboard focus on the
// lesson that was just acted on (or the day it left) instead of dumping it
// onto the document body.
(function () {
    "use strict";

    function requestPath(event) {
        var detail = event.detail || {};
        if (detail.pathInfo && detail.pathInfo.requestPath) {
            return String(detail.pathInfo.requestPath);
        }
        if (detail.requestConfig && detail.requestConfig.path) {
            return String(detail.requestConfig.path);
        }
        return "";
    }

    function focusPlace(node) {
        if (!node || !node.setAttribute) {
            return;
        }
        node.setAttribute("tabindex", "-1");
        try {
            node.focus({preventScroll: true});
        } catch (e) {
            try { node.focus(); } catch (e2) { /* ignore */ }
        }
    }

    document.body.addEventListener("htmx:afterSettle", function (event) {
        var match = requestPath(event).match(/\/lessons\/(\d+)\//);
        if (!match) {
            return;
        }
        var lesson = document.getElementById("lesson-" + match[1]);
        if (lesson) {
            focusPlace(lesson);
            return;
        }
        var elt = event.detail && event.detail.elt;
        if (elt && elt.classList && elt.classList.contains("day")) {
            focusPlace(elt);
        }
    });
})();
