import * as pdfjsLib from "/static/pdfjs/pdf.min.mjs";

pdfjsLib.GlobalWorkerOptions.workerSrc = "/static/pdfjs/pdf.worker.min.mjs";

function clamp(n, lo, hi) {
  return Math.max(lo, Math.min(hi, n));
}

function parseIntAttr(el, name, fallback) {
  var raw = el.getAttribute(name);
  if (raw == null || raw === "") {
    return fallback;
  }
  var n = parseInt(raw, 10);
  return Number.isFinite(n) ? n : fallback;
}

async function mountViewer(root) {
  var url = root.getAttribute("data-file-url");
  if (!url) {
    return;
  }

  var canvas = root.querySelector("[data-pdf-canvas]");
  var stage = root.querySelector("[data-pdf-stage]");
  var status = root.querySelector("[data-pdf-status]");
  var pageLabel = root.querySelector("[data-pdf-page-label]");
  var previousButton = root.querySelector("[data-pdf-prev]");
  var nextButton = root.querySelector("[data-pdf-next]");
  var stageWrap = stage?.parentElement;
  if (!canvas || !stage) {
    return;
  }
  var ctx = canvas.getContext("2d");

  var fitMode = "width"; // width | page | custom
  var customScale = 1;
  var pageStart = Math.max(1, parseIntAttr(root, "data-page-start", 1));
  var pageEndAttr = parseIntAttr(root, "data-page-end", 0);
  var currentPage = pageStart;
  var rendering = false;
  var pendingPage = null;
  var pdfDoc = null;
  var pageEnd = pageStart;
  var currentScale = 1;
  var overlayOpen = false;
  var overlayOpenedAt = 0;
  var overlayRestore = null;
  var pointers = new Map();
  var drag = null;
  var pinching = false;
  var pinchStartDistance = 0;
  var pinchStartScale = 1;
  var pinchTargetScale = 1;
  var gestureMoved = false;

  function setStatus(msg) {
    if (status) {
      status.textContent = msg || "";
    }
  }

  function updateChrome() {
    if (pageLabel) {
      pageLabel.textContent = "Page " + currentPage + " of " + pageEnd +
        (pageStart > 1 || pageEndAttr > 0 ? " (lesson " + pageStart + "–" + pageEnd + ")" : "");
    }
    if (previousButton) {
      previousButton.disabled = currentPage <= pageStart;
    }
    if (nextButton) {
      nextButton.disabled = currentPage >= pageEnd;
    }
    var jumpInput = root.querySelector("[data-pdf-jump]");
    if (jumpInput) {
      jumpInput.value = String(currentPage);
    }
  }

  function computeFitScale(page) {
    var base = page.getViewport({ scale: 1 });
    var availW = Math.max(120, stage.clientWidth - 8);
    var availH = Math.max(120, stage.clientHeight - 8);
    if (fitMode === "page") {
      return Math.min(availW / base.width, availH / base.height);
    }
    if (fitMode === "width") {
      return availW / base.width;
    }
    return customScale;
  }

  async function renderPage(num) {
    if (!pdfDoc) {
      return;
    }
    if (rendering) {
      pendingPage = num;
      return;
    }
    rendering = true;
    try {
      var targetPage = clamp(num, pageStart, pageEnd);
      if (targetPage !== currentPage) {
        stage.scrollLeft = 0;
        stage.scrollTop = 0;
      }
      currentPage = targetPage;
      updateChrome();
      var page = await pdfDoc.getPage(currentPage);
      var scale = computeFitScale(page);
      currentScale = scale;
      var viewport = page.getViewport({ scale: scale });
      var outputScale = window.devicePixelRatio || 1;
      canvas.width = Math.floor(viewport.width * outputScale);
      canvas.height = Math.floor(viewport.height * outputScale);
      canvas.style.width = Math.floor(viewport.width) + "px";
      canvas.style.height = Math.floor(viewport.height) + "px";
      var transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null;
      await page.render({
        canvasContext: ctx,
        viewport: viewport,
        transform: transform
      }).promise;
      updateFullscreenArrowPositions();
      setStatus("");
    } catch (err) {
      setStatus("Could not render that page.");
      console.error(err);
    } finally {
      rendering = false;
      if (pendingPage != null) {
        var next = pendingPage;
        pendingPage = null;
        renderPage(next);
      } else {
        pendingPage = null;
      }
    }
  }

  function updateFullscreenArrowPositions() {
    if (!stageWrap) {
      return;
    }
    if (!overlayOpen) {
      stageWrap.style.removeProperty("--pdf-previous-arrow-left");
      stageWrap.style.removeProperty("--pdf-next-arrow-left");
      return;
    }
    var rect = canvas.getBoundingClientRect();
    var arrowSize = 48;
    var gap = 12;
    var edge = 12;
    var maxLeft = Math.max(edge, window.innerWidth - arrowSize - edge);
    var previousLeft = clamp(rect.left - arrowSize - gap, edge, maxLeft);
    var nextLeft = clamp(rect.right + gap, edge, maxLeft);
    stageWrap.style.setProperty("--pdf-previous-arrow-left", previousLeft + "px");
    stageWrap.style.setProperty("--pdf-next-arrow-left", nextLeft + "px");
  }

  function openOverlay() {
    if (overlayOpen || !pdfDoc) {
      return;
    }
    overlayOpen = true;
    overlayOpenedAt = Date.now();
    overlayRestore = { fitMode: fitMode, customScale: customScale };
    stage.classList.add("is-overlay");
    stage.setAttribute("role", "dialog");
    stage.setAttribute("aria-modal", "true");
    stage.setAttribute("aria-label", "Zoomed PDF page");
    document.body.classList.add("pdf-overlay-open");
    fitMode = "custom";
    customScale = clamp(Math.max(currentScale * 1.5, 1.5), 0.25, 6);
    requestAnimationFrame(function () {
      updateFullscreenArrowPositions();
      renderPage(currentPage);
      overlayClose.focus({ preventScroll: true });
    });
  }

  function closeOverlay() {
    if (!overlayOpen) {
      return;
    }
    overlayOpen = false;
    stage.classList.remove("is-overlay");
    stage.removeAttribute("role");
    stage.removeAttribute("aria-modal");
    stage.removeAttribute("aria-label");
    document.body.classList.remove("pdf-overlay-open");
    updateFullscreenArrowPositions();
    if (overlayRestore) {
      fitMode = overlayRestore.fitMode;
      customScale = overlayRestore.customScale;
    }
    overlayRestore = null;
    canvas.style.transform = "";
    requestAnimationFrame(function () {
      renderPage(currentPage);
    });
  }

  function pointerDistance() {
    var points = Array.from(pointers.values());
    if (points.length < 2) {
      return 0;
    }
    return Math.hypot(points[0].x - points[1].x, points[0].y - points[1].y);
  }

  stage.addEventListener("pointerdown", function (event) {
    if (event.target !== canvas) {
      return;
    }
    event.preventDefault();
    stage.setPointerCapture(event.pointerId);
    pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    if (pointers.size === 1) {
      drag = {
        x: event.clientX,
        y: event.clientY,
        left: stage.scrollLeft,
        top: stage.scrollTop
      };
      gestureMoved = false;
      stage.classList.add("is-grabbing");
    } else if (pointers.size === 2) {
      pinching = true;
      gestureMoved = true;
      pinchStartDistance = Math.max(1, pointerDistance());
      pinchStartScale = currentScale;
      pinchTargetScale = currentScale;
      drag = null;
    }
  });

  stage.addEventListener("pointermove", function (event) {
    if (!pointers.has(event.pointerId)) {
      return;
    }
    event.preventDefault();
    pointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    if (pinching && pointers.size >= 2) {
      var factor = pointerDistance() / pinchStartDistance;
      pinchTargetScale = clamp(pinchStartScale * factor, 0.25, 6);
      canvas.style.transformOrigin = "top left";
      canvas.style.transform = "scale(" + (pinchTargetScale / currentScale) + ")";
      return;
    }
    if (drag && pointers.size === 1) {
      var dx = event.clientX - drag.x;
      var dy = event.clientY - drag.y;
      if (Math.abs(dx) > 4 || Math.abs(dy) > 4) {
        gestureMoved = true;
      }
      stage.scrollLeft = drag.left - dx;
      stage.scrollTop = drag.top - dy;
    }
  });

  function finishPointer(event, cancelled) {
    if (!pointers.has(event.pointerId)) {
      return;
    }
    event.preventDefault();
    pointers.delete(event.pointerId);
    if (pinching && pointers.size < 2) {
      pinching = false;
      canvas.style.transform = "";
      fitMode = "custom";
      customScale = pinchTargetScale;
      renderPage(currentPage);
    }
    if (pointers.size === 0) {
      stage.classList.remove("is-grabbing");
      if (!cancelled && drag && !gestureMoved && !overlayOpen) {
        openOverlay();
      }
      drag = null;
      gestureMoved = false;
    }
  }

  stage.addEventListener("pointerup", function (event) {
    finishPointer(event, false);
  });
  stage.addEventListener("pointercancel", function (event) {
    finishPointer(event, true);
  });
  stage.addEventListener("click", function (event) {
    if (overlayOpen && event.target === stage && Date.now() - overlayOpenedAt > 250) {
      closeOverlay();
    }
  });
  stage.addEventListener("scroll", function () {
    if (overlayOpen) {
      requestAnimationFrame(updateFullscreenArrowPositions);
    }
  });

  var overlayClose = document.createElement("button");
  overlayClose.type = "button";
  overlayClose.className = "pdf-viewer-overlay-close";
  overlayClose.setAttribute("aria-label", "Close zoomed PDF");
  overlayClose.textContent = "×";
  overlayClose.addEventListener("click", closeOverlay);
  stage.appendChild(overlayClose);

  document.addEventListener("keydown", function (event) {
    if (overlayOpen && event.key === "Escape") {
      closeOverlay();
    }
  });

  root.querySelector("[data-pdf-prev]")?.addEventListener("click", function () {
    renderPage(currentPage - 1);
  });
  root.querySelector("[data-pdf-next]")?.addEventListener("click", function () {
    renderPage(currentPage + 1);
  });
  root.querySelector("[data-pdf-zoom-in]")?.addEventListener("click", function () {
    fitMode = "custom";
    customScale = clamp((customScale || 1) * 1.2, 0.25, 6);
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-zoom-out]")?.addEventListener("click", function () {
    fitMode = "custom";
    customScale = clamp((customScale || 1) / 1.2, 0.25, 6);
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-fit-width]")?.addEventListener("click", function () {
    fitMode = "width";
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-fit-page]")?.addEventListener("click", function () {
    fitMode = "page";
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-reset]")?.addEventListener("click", function () {
    fitMode = "width";
    customScale = 1;
    renderPage(pageStart);
  });

  var jump = root.querySelector("[data-pdf-jump]");
  if (jump) {
    jump.addEventListener("change", function () {
      var n = parseInt(jump.value, 10);
      if (Number.isFinite(n)) {
        renderPage(n);
      }
    });
  }

  window.addEventListener("resize", function () {
    if (fitMode === "width" || fitMode === "page") {
      renderPage(currentPage);
    }
  });

  setStatus("Loading PDF…");
  try {
    pdfDoc = await pdfjsLib.getDocument({
      url: url,
      withCredentials: true,
      // Hosted connections pay much more per request than LAN connections.
      // Larger ranges avoid hundreds of 64 KiB round trips for image-heavy
      // textbooks, while these flags prevent downloading the whole book in
      // the background when the lesson only needs a few pages.
      rangeChunkSize: 1024 * 1024,
      disableAutoFetch: true,
      disableStream: true
    }).promise;
    if (pageEndAttr > 0) {
      pageEnd = Math.min(pageEndAttr, pdfDoc.numPages);
    } else {
      pageEnd = pdfDoc.numPages;
    }
    pageStart = clamp(pageStart, 1, pdfDoc.numPages);
    pageEnd = clamp(pageEnd, pageStart, pdfDoc.numPages);
    currentPage = pageStart;
    if (jump) {
      jump.min = String(pageStart);
      jump.max = String(pageEnd);
      jump.value = String(pageStart);
    }
    await renderPage(currentPage);
  } catch (err) {
    setStatus("Could not open that PDF.");
    console.error(err);
  }
}

document.querySelectorAll("[data-pdf-viewer]").forEach(function (el) {
  mountViewer(el);
});
