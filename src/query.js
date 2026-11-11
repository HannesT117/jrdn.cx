import maxChars from "../api/internal/httpapi/limits.json";

const endpoint = "/api/query";
const node = document.querySelector(".n-query");
const form = document.getElementById("query-form");
const text = document.getElementById("query-text");
const count = document.getElementById("query-count");
const result = document.getElementById("query-result");

const fromTemplate = (id) => document.getElementById(id).content.cloneNode(true);

const show = (state, content) => {
    node.dataset.state = state;
    result.setAttribute("aria-busy", String(state === "loading"));
    result.replaceChildren(content);
};

const updateCount = () => {
    count.value = `${text.value.length} / ${text.maxLength}`;
};

const applyMode = () => {
    const mode = new FormData(form).get("mode");
    text.maxLength = maxChars[mode];
    text.placeholder = text.dataset[mode === "fit" ? "placeholderFit" : "placeholderAsk"];
    updateCount();
};

const speech = new Audio();

const setSpeaking = (button, state) => {
    button.dataset.state = state;
    button.setAttribute("aria-pressed", String(state === "playing"));
    button.querySelector(".speak-label").textContent =
        { loading: "Loading…", playing: "Pause", failed: "Audio unavailable" }[state] ?? "Read aloud";
};

const wireSpeech = (button, id) => {
    const src = `/api/speech/${encodeURIComponent(id)}`;
    speech.pause();
    speech.onplaying = () => setSpeaking(button, "playing");
    speech.onpause = speech.onended = () => setSpeaking(button, "idle");
    speech.onerror = () => setSpeaking(button, "failed");
    button.addEventListener("click", () => {
        if (!speech.paused) return speech.pause();
        if (!speech.src.endsWith(src)) {
            setSpeaking(button, "loading");
            speech.src = src;
        }
        speech.play().catch(() => setSpeaking(button, "failed"));
    });
};

const renderAnswer = (question, { id, text }) => {
    const view = fromTemplate("t-answer");
    view.querySelector(".asked").textContent = `“${question}”`;
    view.querySelector(".answer").textContent = text;
    wireSpeech(view.querySelector(".speak"), id);
    return view;
};

const renderFit = ({ matches = [], learning = [], strong = false }) => {
    const view = fromTemplate("t-fit");
    if (!strong) view.querySelector(".strong").remove();
    const list = view.querySelector(".matches");
    for (const label of matches) {
        const item = document.createElement("li");
        item.textContent = label;
        list.append(item);
    }
    if (learning.length === 0) {
        view.querySelector(".learning-lead").remove();
        view.querySelector(".learning").remove();
    }
    for (const label of learning) {
        const item = document.createElement("li");
        item.textContent = label;
        view.querySelector(".learning").append(item);
    }
    return view;
};

const render = (question, data) => {
    switch (data.kind) {
        case "answer":
            return ["done", renderAnswer(question, data)];
        case "no_answer":
            return ["done", fromTemplate("t-no-answer")];
        case "fit":
            return ["done", renderFit(data)];
        case "lets_talk":
            return ["done", fromTemplate("t-lets-talk")];
        case "unavailable":
            return ["unavailable", fromTemplate("t-unavailable")];
        default:
            return ["error", fromTemplate("t-error")];
    }
};

const submit = async (event) => {
    event.preventDefault();
    text.setCustomValidity(text.value.trim() ? "" : "Please write a question or paste a role.");
    if (node.dataset.state === "loading" || !form.reportValidity()) return;

    const body = Object.fromEntries(new FormData(form));
    show("loading", fromTemplate("t-loading"));

    try {
        const response = await fetch(endpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body),
            signal: AbortSignal.timeout(20_000),
        });
        const data = response.headers.get("Content-Type")?.includes("application/json")
            ? await response.json()
            : { kind: "unavailable" };
        show(...render(body.text.trim(), data));
    } catch {
        show("error", fromTemplate("t-error"));
    }
};

node.addEventListener("change", applyMode);
text.addEventListener("input", updateCount);
form.addEventListener("submit", submit);
applyMode();
