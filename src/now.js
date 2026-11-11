const HOUR = 3_600_000;
const DAYS = 4;
const ZONE = "Australia/Sydney";
const SLOTS = 4;
const KINDS = ["run", "ride", "other"];
const KIND_LABEL = { run: "Run", ride: "Ride", other: "Workout" };
const kindOf = (kind) => (KINDS.includes(kind) ? kind : "other");

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};

const hue = (node, text) => {
    let h = 0;
    for (const char of text) h = (h * 31 + char.codePointAt(0)) % 360;
    node.style.setProperty("--h", h);
    return node;
};

const sydney = new Intl.DateTimeFormat("en-AU", { timeZone: ZONE, hourCycle: "h23", year: "numeric", month: "numeric", day: "numeric", hour: "numeric", minute: "numeric", second: "numeric" });

function midnights(from, to) {
    const out = [];
    let previous = sydney.format(from).split(",")[0];
    for (let t = from + HOUR; t <= to; t += HOUR) {
        const parts = Object.fromEntries(sydney.formatToParts(t).map((p) => [p.type, Number(p.value)]));
        const day = sydney.format(t).split(",")[0];
        if (day !== previous) out.push(t - ((parts.hour * 60 + parts.minute) * 60 + parts.second) * 1000 - (t % 1000));
        previous = day;
    }
    return out;
}

const weekday = (ms) => new Date(ms).toLocaleDateString("en-AU", { weekday: "short", timeZone: ZONE });
const month = (date) => new Date(date).toLocaleDateString("en-AU", { month: "short", timeZone: "UTC" });

const load = async (url) => {
    try {
        const res = await fetch(url, { signal: AbortSignal.timeout(8_000) });
        return res.ok ? await res.json() : null;
    } catch {
        return null;
    }
};

function fill(feed, part, items, render) {
    if (!items?.length) return;
    const box = feed.querySelector(`[data-part="${part}"]`);
    render(box, items);
    box.hidden = false;
    feed.hidden = false;
}

const plate = (box, items) => {
    box.querySelector("dl").append(
        ...[...Map.groupBy(items, (it) => it.kind)].flatMap(([kind, group]) => [
            el("dt", { textContent: kind }),
            ...group.map((it) => el("dd", {}, it.name, ...(it.details?.length ? [el("small", { textContent: it.details.join(" · ") })] : []))),
        ]),
    );
};

const log = (box, items, text, note) => {
    box.querySelector(".log").append(
        ...items.map((it) => hue(el("li", {}, el("span", { textContent: text(it) }), el("small", { textContent: note?.(it) ?? "" })), text(it))),
    );
};

function renderActivity(box, activities) {
    const now = Date.now();
    const around = midnights(now - (DAYS + 1) * 24 * HOUR, now + 26 * HOUR);
    const today = around.findLastIndex((m) => m <= now);
    const bounds = around.slice(today - DAYS + 1, today + 2);
    const [start, end] = [bounds[0], bounds.at(-1)];
    const last = activities[0];
    const state = box.querySelector(".state");
    if (last) {
        const hoursAgo = Math.max(0, Math.round((now - Date.parse(last.start) - last.minutes * 60_000) / HOUR));
        const level = hoursAgo < 24 ? "ok" : hoursAgo < 36 ? "warn" : "down";
        state.className = `state s-${level}`;
        state.textContent = `${{ ok: "operational", warn: "degraded", down: "outage" }[level]} · last activity ${hoursAgo}h ago`;
    } else {
        state.className = "state s-down";
        state.textContent = `outage · nothing in ${DAYS} days`;
    }

    const span = end - start;
    const at = (ms) => ((ms - start) / span).toFixed(4);
    const track = box.querySelector(".track");
    const cuts = bounds.slice(1, -1);
    const active = [];
    for (let i = 0; i < bounds.length - 1; i++) {
        const [from, to] = [bounds[i], bounds[i + 1]];
        const middle = (from + to) / 2;
        const kinds = KINDS.filter((k) => activities.some((a) => kindOf(a.kind) === k && Date.parse(a.start) >= from && Date.parse(a.start) < to));
        if (kinds.length) {
            const day = el("span", { className: "day" }, ...kinds.map((k) => el("i", { className: `t-${k}` })));
            day.style.setProperty("--from", at(from));
            day.style.setProperty("--len", ((to - from) / span).toFixed(4));
            track.append(day);
            active.push(`${weekday(middle)} ${kinds.map((k) => KIND_LABEL[k].toLowerCase()).join(" and ")}`);
        }
        const label = el("span", { textContent: weekday(middle) });
        label.style.setProperty("--at", at(middle));
        box.querySelector(".days").append(label);
    }
    for (const m of cuts) {
        const tick = el("span", { className: "tick" });
        tick.style.setProperty("--at", at(m));
        track.append(tick);
    }
    for (const className of ["later", "now"]) {
        const mark = el("span", { className });
        mark.style.setProperty("--at", at(now));
        track.append(mark);
    }
    track.setAttribute("aria-label", active.length ? `Active in the last ${DAYS} days: ${active.join(", ")}` : `No activity in the last ${DAYS} days`);
    box.querySelector(".kinds").append(...KINDS.map((k) => el("li", { className: `t-${k}` }, KIND_LABEL[k])));
}

function renderSport(extras, activity) {
    const feed = document.getElementById("feed-sport");
    if (activity) {
        const box = feed.querySelector('[data-part="activity"]');
        renderActivity(box, activity.activities);
        box.hidden = false;
        feed.hidden = false;
    }
    fill(feed, "bikes", extras?.bikes, plate);
    fill(feed, "sport-gear", extras?.sportGear, plate);
}

function renderCoffee(extras) {
    if (!extras) return;
    const feed = document.getElementById("feed-coffee");
    fill(feed, "beans", [extras.beans ?? []], (box, [beans]) => {
        const slots = Array.from({ length: SLOTS }, (_, i) => beans[i]);
        box.querySelector(".hoppers").append(
            ...slots.map((b) =>
                b
                    ? el(
                          "li",
                          { className: "hopper" },
                          el("b", { textContent: b.name }),
                          el("small", { textContent: [b.origin, b.process].filter(Boolean).join(" · ") }),
                          el("small", { textContent: b.roaster }),
                      )
                    : el("li", { className: "hopper empty" }, el("small", { textContent: "free slot" })),
            ),
        );
    });
    fill(feed, "brews", extras.brews, (box, brews) => {
        box.querySelector(".chips").append(...brews.map((b) => el("li", { textContent: b })));
    });
    fill(feed, "gear", extras.gear, plate);
    fill(feed, "past-beans", extras.pastBeans, (box, beans) => log(box, beans, (b) => `${b.name} · ${b.roaster}`, (b) => b.origin));
}

function renderReading(extras) {
    if (!extras) return;
    const feed = document.getElementById("feed-reading");
    fill(feed, "books", extras.books, (box, books) => {
        box.querySelector(".books").append(
            ...books.map((b) => hue(el("li", {}, el("cite", { textContent: b.title }), el("span", { textContent: b.author })), b.title)),
        );
    });
    fill(feed, "past-books", extras.pastBooks, (box, books) => log(box, books, (b) => `${b.title} · ${b.author}`, (b) => month(b.finished)));
    fill(feed, "podcasts", extras.podcasts, (box, podcasts) => log(box, podcasts, (p) => [p.name, p.host].filter(Boolean).join(" · ")));
}

function renderMusic(extras) {
    if (!extras) return;
    const feed = document.getElementById("feed-music");
    fill(feed, "repertoire", extras.repertoire, (box, pieces) => {
        new IntersectionObserver(
            async (entries, observer) => {
                if (!entries.some((e) => e.isIntersecting)) return;
                observer.disconnect();
                const [{ mountCloud }] = await Promise.all([import("./cloud.js"), document.fonts.ready]);
                mountCloud(box, pieces);
            },
            { rootMargin: "400px" },
        ).observe(box);
    });
    fill(feed, "stages", extras.stages, (box, stages) => {
        box.querySelector(".tally").textContent = extras.concerts ? `Around ${extras.concerts} concerts. A few of them:` : "A few concerts:";
        box.querySelector(".stages").append(
            ...stages.map((st) =>
                el(
                    "li",
                    {},
                    el("span", {}, el("b", { textContent: st.title }), st.place ? `, ${st.place}` : ""),
                    el("small", { textContent: st.year ?? "" }),
                    ...(st.with ? [el("small", { className: "with", textContent: st.with })] : []),
                ),
            ),
        );
    });
    fill(feed, "countries", extras.countries, (box, countries) => {
        box.querySelector(".chips").append(...countries.toSorted((a, b) => a.localeCompare(b, "en")).map((c) => el("li", { textContent: c })));
    });
    fill(feed, "music-kit", extras.music, plate);
    fill(feed, "gigs", extras.gigs, (box, gigs) => {
        box.querySelector(".gigs").append(
            ...gigs.map((g) => {
                const day = new Date(`${g.date}T00:00:00Z`);
                return el(
                    "li",
                    {},
                    el("time", { dateTime: g.date }, el("b", { textContent: day.getUTCDate() }), month(day)),
                    el("div", {}, el("cite", { textContent: g.program }), el("span", { textContent: [g.choir, g.place].filter(Boolean).join(" · ") })),
                );
            }),
        );
    });
}

const [extras, activity] = await Promise.all([load("/api/extras"), load("/api/activity")]);
renderSport(extras, activity);
renderCoffee(extras);
renderReading(extras);
renderMusic(extras);
