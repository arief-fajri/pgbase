<template>
  <div class="doc-meta" role="note">
    <span v-if="audience" class="doc-meta-badge doc-meta-audience">
      <span class="doc-meta-key">Audience</span>
      <span class="doc-meta-val">{{ audience }}</span>
    </span>
    <span v-if="status" class="doc-meta-badge doc-meta-status" :data-tone="statusTone">
      <span class="doc-meta-dot" aria-hidden="true"></span>
      <span class="doc-meta-val">{{ status }}</span>
    </span>
    <span v-if="verified" class="doc-meta-badge doc-meta-verified">
      <span class="doc-meta-key">Verified</span>
      <code class="doc-meta-val">{{ verified }}</code>
    </span>
  </div>
</template>

<script setup>
import { computed } from "vue";

const props = defineProps({
  audience: { type: String, default: "" },
  status: { type: String, default: "" },
  verified: { type: String, default: "" },
});

const statusTone = computed(() => {
  const s = props.status.toLowerCase();
  if (s.includes("experimental") || s.includes("draft") || s.includes("wip")) return "warn";
  if (s.includes("deprecated") || s.includes("removed")) return "danger";
  if (s.includes("living")) return "info";
  return "ok";
});
</script>

<style scoped>
.doc-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin: 16px 0 24px;
}

.doc-meta-badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 9px;
  font-size: 11px;
  line-height: 16px;
  font-weight: 500;
  border-radius: 10px;
  border: 1px solid var(--vp-c-divider);
  background: var(--vp-c-bg-soft);
  color: var(--vp-c-text-2);
}

.doc-meta-key {
  color: var(--vp-c-text-3);
  font-weight: 600;
  text-transform: uppercase;
  font-size: 9px;
  letter-spacing: 0.4px;
}

.doc-meta-val {
  color: var(--vp-c-text-1);
}

.doc-meta-audience {
  border-color: var(--vp-c-brand-3);
}

.doc-meta-verified code {
  background: transparent;
  padding: 0;
  font-size: 11px;
  color: var(--vp-c-brand-1);
}

.doc-meta-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--vp-c-green-1);
}

.doc-meta-status[data-tone="warn"] .doc-meta-dot {
  background: var(--vp-c-yellow-1);
}

.doc-meta-status[data-tone="danger"] .doc-meta-dot {
  background: var(--vp-c-red-1);
}

.doc-meta-status[data-tone="info"] .doc-meta-dot {
  background: var(--vp-c-brand-1);
}
</style>
