<template>
  <div ref="root" class="mermaid-viewer" :class="{ 'is-fullscreen': isFullscreen }">
    <div v-if="showToolbar" class="mermaid-toolbar">
      <button type="button" title="Zoom in" @click="zoomBy(1.3)">＋</button>
      <button type="button" title="Zoom out" @click="zoomBy(1 / 1.3)">－</button>
      <button type="button" title="Reset view" @click="reset">⟳</button>
      <button
        type="button"
        :title="isFullscreen ? 'Exit fullscreen' : 'Fullscreen'"
        @click="toggleFullscreen"
      >
        {{ isFullscreen ? "⤢" : "⤡" }}
      </button>
    </div>
    <div ref="viewport" class="mermaid-canvas">
      <Suspense>
        <template #default>
          <Mermaid v-if="elkReady" :id="id" :class="class" :graph="graph" />
        </template>
        <template #fallback>Loading…</template>
      </Suspense>
    </div>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref } from "vue";
import { zoom as d3zoom, zoomIdentity, zoomTransform } from "d3-zoom";
import { select } from "d3-selection";
import { ensureElkRegistered } from "./registerElk";

const props = defineProps({
  id: { type: String, required: true },
  graph: { type: String, required: true },
  class: { type: String, default: "mermaid" },
  showToolbar: { type: Boolean, default: true },
});

const root = ref(null);
const viewport = ref(null);
const isFullscreen = ref(false);
// Gate the plugin's <Mermaid> mount until the ELK layout loader is registered,
// so mermaid.render() never runs before "elk" exists (else it falls back to
// dagre). SSR renders nothing; the client resolves this before first paint.
const elkReady = ref(false);

let zoomBehavior = null;
let observer = null;
let hasFitted = false;

const SCALE_MIN = 0.4;
const SCALE_MAX = 8;

const getSvg = () => viewport.value?.querySelector("svg") ?? null;

// Apply a d3 zoom transform as a CSS transform on the SVG. We build the string
// with explicit `px` units instead of transform.toString(): d3 emits unitless
// `translate(x,y)` (valid for the SVG transform *attribute*), but the CSS
// `translate()` function requires length units, so an unitless value is invalid
// and silently dropped. CSS transform works for every diagram type (no inner-<g>
// assumption), unlike the transform attribute which the root <svg> ignores.
const applyTransform = (t) => {
  const svg = getSvg();
  if (!svg) return;
  svg.style.transformOrigin = "0 0";
  svg.style.transform = `translate(${t.x}px, ${t.y}px) scale(${t.k})`;
};

const onZoom = (event) => applyTransform(event.transform);

const selection = () => select(viewport.value);

// Buttons: multiplicative scale about the viewport centre. scaleBy respects
// scaleExtent, so it clamps at SCALE_MIN/MAX for free.
const zoomBy = (factor) => {
  if (!zoomBehavior || !viewport.value) return;
  zoomBehavior.scaleBy(selection(), factor);
};

// Centre + scale-to-fit (shrink-only, never upscale) inside the viewport.
const fit = () => {
  const svg = getSvg();
  const vp = viewport.value;
  if (!svg || !vp || !zoomBehavior) return;

  // Measure the SVG's natural (untransformed) box.
  const prev = svg.style.transform;
  svg.style.transform = "none";
  const rect = svg.getBoundingClientRect();
  const nw = rect.width;
  const nh = rect.height;
  svg.style.transform = prev;
  if (!nw || !nh) return;

  const vw = vp.clientWidth;
  const vh = vp.clientHeight;
  const k = Math.max(SCALE_MIN, Math.min(vw / nw, vh / nh, 1));
  const tx = (vw - nw * k) / 2;
  const ty = (vh - nh * k) / 2;

  zoomBehavior.transform(
    selection(),
    zoomIdentity.translate(tx, ty).scale(k)
  );
};

const reset = () => fit();

// A new SVG node appears on first render and again on every theme toggle
// (the plugin re-renders via v-html + a random salt). Keep the listener on the
// un-replaced viewport <div>; here we only (re)apply the transform to the new
// node — fit on first sight, otherwise preserve the user's current zoom.
const processSvg = () => {
  const svg = getSvg();
  if (!svg || svg.dataset.pzInit === "1") return;
  svg.dataset.pzInit = "1";
  svg.style.transformOrigin = "0 0";
  if (!hasFitted) {
    hasFitted = true;
    requestAnimationFrame(fit);
  } else {
    applyTransform(zoomTransform(viewport.value));
  }
};

const onFullscreenChange = () => {
  isFullscreen.value = document.fullscreenElement === root.value;
  nextTick(() => requestAnimationFrame(fit));
};

const toggleFullscreen = async () => {
  if (document.fullscreenElement === root.value) {
    await document.exitFullscreen?.();
  } else {
    await root.value?.requestFullscreen?.();
  }
};

onMounted(async () => {
  await ensureElkRegistered();
  elkReady.value = true;

  zoomBehavior = d3zoom()
    .scaleExtent([SCALE_MIN, SCALE_MAX])
    .on("zoom", onZoom);

  selection().call(zoomBehavior).on("dblclick.zoom", null);

  observer = new MutationObserver(processSvg);
  observer.observe(viewport.value, { childList: true, subtree: true });

  // Catch an SVG that is already present by the time we attach the observer.
  await nextTick();
  processSvg();

  document.addEventListener("fullscreenchange", onFullscreenChange);
});

onBeforeUnmount(() => {
  document.removeEventListener("fullscreenchange", onFullscreenChange);
  observer?.disconnect();
  if (viewport.value) selection().on(".zoom", null);
});
</script>

<style scoped>
.mermaid-viewer {
  position: relative;
  margin: 1.5rem 0;
}

.mermaid-toolbar {
  position: absolute;
  top: 8px;
  right: 8px;
  z-index: 10;
  display: flex;
  gap: 4px;
  padding: 4px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  background: var(--vp-c-bg-soft);
}

.mermaid-toolbar button {
  width: 30px;
  height: 30px;
  border: 1px solid var(--vp-c-divider);
  border-radius: 6px;
  background: var(--vp-c-bg);
  color: var(--vp-c-text-1);
  cursor: pointer;
  line-height: 1;
  font-size: 16px;
}

.mermaid-toolbar button:hover {
  border-color: var(--vp-c-brand-1);
  color: var(--vp-c-brand-1);
}

.mermaid-canvas {
  position: relative;
  overflow: hidden;
  height: var(--mermaid-viewer-h, 460px);
  background: var(--vp-c-bg-soft);
  border: 1px solid var(--vp-c-divider);
  border-radius: 8px;
  cursor: grab;
  /* Route wheel / pinch / drag gestures to d3-zoom, not the page. */
  touch-action: none;
}

.mermaid-canvas:active {
  cursor: grabbing;
}

.mermaid-canvas :deep(svg) {
  transform-origin: 0 0;
  max-width: none;
  display: block;
}

/* VitePress `.vp-doc p` forces a 28px line-height onto mermaid htmlLabels, but
   mermaid sizes each <foreignObject> at its own 1.5 (=24px) basis, so the extra
   4px/line overflows and the SVG clips the bottom line of every multi-line node.
   Pin the label back to mermaid's basis so rendered height == measured height. */
.mermaid-canvas :deep(foreignObject) div,
.mermaid-canvas :deep(foreignObject) .nodeLabel,
.mermaid-canvas :deep(foreignObject) p {
  line-height: 1.5 !important;
  margin: 0 !important;
  padding: 0 !important;
}

.mermaid-viewer.is-fullscreen {
  display: flex;
  flex-direction: column;
  background: var(--vp-c-bg);
}

.mermaid-viewer.is-fullscreen .mermaid-canvas {
  flex: 1;
  height: auto;
  border: 0;
  border-radius: 0;
}
</style>
