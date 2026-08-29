// Theme switching: automatic (follow the computer), light, or dark.
//
// The document head has already resolved and applied the theme by the time
// this runs. This file only handles the button and keeps "automatic" honest
// when the computer changes its mind.
(function () {
    "use strict";

    var STORAGE_KEY = "school-nanny-theme";
    var MODES = ["auto", "light", "dark"];
    var LABELS = { auto: "Auto", light: "Light", dark: "Dark" };

    var root = document.documentElement;
    var prefersDark = window.matchMedia("(prefers-color-scheme: dark)");
    var mode = readMode();

    function readMode() {
        var saved = null;
        try {
            saved = localStorage.getItem(STORAGE_KEY);
        } catch (e) {
            // Storage can be unavailable in private windows; automatic is fine.
        }
        return saved === "light" || saved === "dark" ? saved : "auto";
    }

    function saveMode(next) {
        try {
            if (next === "auto") {
                localStorage.removeItem(STORAGE_KEY);
            } else {
                localStorage.setItem(STORAGE_KEY, next);
            }
        } catch (e) {
            // Not being able to remember the choice should not break the page.
        }
    }

    function apply(next) {
        var dark = next === "dark" || (next === "auto" && prefersDark.matches);
        root.setAttribute("data-theme", dark ? "dark" : "light");
        root.setAttribute("data-theme-mode", next);

        var buttons = document.querySelectorAll("[data-theme-toggle]");
        for (var i = 0; i < buttons.length; i++) {
            buttons[i].setAttribute("data-mode", next);
            var label = buttons[i].querySelector("[data-theme-label]");
            if (label) {
                label.textContent = LABELS[next];
            }
        }
    }

    apply(mode);

    // Delegated so the button keeps working after HTMX replaces part of a page.
    document.addEventListener("click", function (event) {
        var button = event.target.closest && event.target.closest("[data-theme-toggle]");
        if (!button) {
            return;
        }
        mode = MODES[(MODES.indexOf(mode) + 1) % MODES.length];
        saveMode(mode);
        apply(mode);
    });

    function followSystem() {
        if (mode === "auto") {
            apply("auto");
        }
    }

    if (prefersDark.addEventListener) {
        prefersDark.addEventListener("change", followSystem);
    } else if (prefersDark.addListener) {
        prefersDark.addListener(followSystem);
    }
})();
