<template>
  <div ref="host" class="json-editor"></div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from "vue";
import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView,
  keymap,
  highlightActiveLine,
  highlightActiveLineGutter,
  lineNumbers,
} from "@codemirror/view";
import {
  defaultKeymap,
  history,
  historyKeymap,
  indentWithTab,
} from "@codemirror/commands";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { lintGutter, linter } from "@codemirror/lint";
import {
  bracketMatching,
  defaultHighlightStyle,
  syntaxHighlighting,
  indentOnInput,
  foldGutter,
  foldKeymap,
} from "@codemirror/language";

const props = defineProps({
  modelValue: { type: String, default: "" },
  readonly: { type: Boolean, default: false },
  /** "auto" — растёт по содержимому, "fixed" — фиксированная высота. */
  height: { type: String, default: "auto" },
});
const emit = defineEmits(["update:modelValue"]);

const host = ref(null);
let view = null;
let editable = new Compartment();
let suppressEmit = false;

onMounted(() => {
  if (!host.value) return;
  view = new EditorView({
    parent: host.value,
    state: EditorState.create({
      doc: props.modelValue || "",
      extensions: [
        lineNumbers(),
        foldGutter(),
        history(),
        bracketMatching(),
        indentOnInput(),
        highlightActiveLine(),
        highlightActiveLineGutter(),
        syntaxHighlighting(defaultHighlightStyle),
        json(),
        // Inline JSON-syntax linter — underlines parser errors and adds a
        // marker to the gutter so юзер сразу видит, где сломалось.
        linter(jsonParseLinter()),
        lintGutter(),
        keymap.of([...defaultKeymap, ...historyKeymap, ...foldKeymap, indentWithTab]),
        editable.of(EditorView.editable.of(!props.readonly)),
        EditorView.updateListener.of((update) => {
          if (!update.docChanged || suppressEmit) return;
          const next = update.state.doc.toString();
          emit("update:modelValue", next);
        }),
        EditorView.theme({
          "&": {
            backgroundColor: "rgba(255, 255, 255, 0.02)",
            border: "1px solid rgba(255, 255, 255, 0.08)",
            borderRadius: "18px",
            color: "#fbfbfb",
            fontSize: "13px",
          },
          "&.cm-focused": {
            outline: "2px solid #1259d1",
            outlineOffset: "0px",
          },
          ".cm-scroller": {
            fontFamily: "'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
            lineHeight: "1.55",
            padding: "12px 4px",
          },
          ".cm-gutters": {
            backgroundColor: "transparent",
            color: "rgba(252, 252, 252, 0.4)",
            border: "none",
            borderRight: "1px solid rgba(255, 255, 255, 0.06)",
          },
          ".cm-activeLineGutter, .cm-activeLine": {
            backgroundColor: "rgba(18, 89, 209, 0.08)",
          },
          ".cm-content": { caretColor: "#fbfbfb" },
          ".cm-lineNumbers .cm-gutterElement": { padding: "0 8px 0 12px" },
          ".cm-cursor": { borderLeftColor: "#fbfbfb" },
          ".cm-tooltip": {
            backgroundColor: "#242427",
            color: "#fbfbfb",
            border: "1px solid rgba(255, 255, 255, 0.08)",
            borderRadius: "8px",
          },
        }),
        ...(props.height === "fixed"
          ? [
              EditorView.theme({
                "&": { height: "420px" },
                ".cm-scroller": { overflow: "auto" },
              }),
            ]
          : []),
      ],
    }),
  });
});

onBeforeUnmount(() => {
  view?.destroy();
  view = null;
});

// Внешние изменения modelValue (например, форма правит JSON и наоборот) —
// перезаливаем содержимое без эмита, чтобы избежать петель.
watch(
  () => props.modelValue,
  (next) => {
    if (!view) return;
    const current = view.state.doc.toString();
    if (current === next) return;
    suppressEmit = true;
    view.dispatch({
      changes: { from: 0, to: current.length, insert: next || "" },
    });
    suppressEmit = false;
  },
);

watch(
  () => props.readonly,
  (next) => {
    if (!view) return;
    view.dispatch({
      effects: editable.reconfigure(EditorView.editable.of(!next)),
    });
  },
);
</script>

<style scoped>
.json-editor {
  display: block;
  width: 100%;
}
</style>
