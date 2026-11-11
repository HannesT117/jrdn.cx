const ERAS = [
    { id: "renaissance", name: "Renaissance", from: 1500 },
    { id: "baroque", name: "Baroque", from: 1600 },
    { id: "classical", name: "Classical", from: 1750 },
    { id: "romantic", name: "Romantic", from: 1820 },
    { id: "modern", name: "Modern", from: 1910 },
];
const END = 2030;
const BIG = new Set(["Mass", "Requiem", "Oratorio", "Passion", "Symphony", "Cantata", "Te Deum", "Chorale cantata"]);
const SPACING = 2.4;
const TUBE = 9;
const GOLDEN = Math.PI * (3 - Math.sqrt(5));
const HOME = { yaw: -0.42, pitch: 0.32 };
const PITCH = 1.35;
const NAME_FONT = "600 11px";
const TITLE_FONT = "12px";

const eraOf = (year) => ERAS[Math.max(0, ERAS.findLastIndex((e) => year >= e.from))];
const yearText = (p) => (p.approx ? `c. ${p.year}` : `${p.year}`);
const median = (xs) => xs.toSorted((a, b) => a - b)[Math.floor(xs.length / 2)];
const clamp = (v, lo, hi) => Math.min(Math.max(v, lo), hi);

const seeded = (seed) => () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
};

function layout(pieces) {
    const random = seeded(117);
    const clusters = [...Map.groupBy(pieces, (p) => p.composer)]
        .map(([name, list]) => {
            const n = list.length;
            const radius = n === 1 ? 0 : Math.max(SPACING / 2, SPACING * Math.sqrt(n / (4 * Math.PI)));
            const dots = list
                .toSorted((a, b) => a.year - b.year)
                .map((piece, i) => {
                    const y = n === 1 ? 0 : 1 - (2 * (i + 0.5)) / n;
                    const ring = Math.sqrt(1 - y * y);
                    return {
                        piece,
                        big: BIG.has(piece.form),
                        off: [Math.cos(i * GOLDEN) * ring * radius, y * radius, Math.sin(i * GOLDEN) * ring * radius],
                    };
                });
            const year = median(list.map((p) => p.year));
            return { name, year, era: eraOf(year), dots, radius, size: radius + 1.3, p: [0, 0, 0] };
        })
        .sort((a, b) => a.year - b.year);

    let cursor = 0;
    const bands = ERAS.map((era, i) => {
        const own = clusters.filter((c) => c.era === era);
        const width = Math.max(8, (2.4 * own.reduce((sum, c) => sum + c.size ** 2, 0)) / TUBE);
        const band = { era, from: cursor, width, until: ERAS[i + 1]?.from ?? END };
        cursor += width + 2;
        return band;
    });
    const shift = (cursor - 2) / 2;
    for (const b of bands) b.from -= shift;

    clusters.forEach((c, i) => {
        const band = bands.find((b) => b.era === c.era);
        c.tx = band.from + ((c.year - band.era.from) / (band.until - band.era.from)) * band.width;
        const rho = TUBE * Math.sqrt(random());
        c.p = [c.tx, Math.cos(i * GOLDEN * 7) * rho, Math.sin(i * GOLDEN * 7) * rho];
    });
    for (let step = 0; step < 300; step++) {
        for (const c of clusters) {
            c.p[0] += (c.tx - c.p[0]) * 0.05;
            const rho = Math.hypot(c.p[1], c.p[2]);
            const limit = TUBE - c.size * 0.5;
            if (rho > limit) {
                const pull = 1 - ((rho - limit) * 0.1) / rho;
                c.p[1] *= pull;
                c.p[2] *= pull;
            }
        }
        for (let i = 0; i < clusters.length; i++) {
            for (let j = i + 1; j < clusters.length; j++) {
                const a = clusters[i];
                const b = clusters[j];
                const d = [0, 1, 2].map((k) => b.p[k] - a.p[k]);
                const dist = Math.hypot(...d) || 0.01;
                const overlap = a.size + b.size - dist;
                if (overlap <= 0) continue;
                const wa = b.size / (a.size + b.size);
                for (let k = 0; k < 3; k++) {
                    a.p[k] -= (d[k] / dist) * overlap * wa;
                    b.p[k] += (d[k] / dist) * overlap * (1 - wa);
                }
            }
        }
    }

    const dots = clusters.flatMap((c) => c.dots.map((d) => Object.assign(d, { cluster: c, p: d.off.map((v, k) => v + c.p[k]) })));
    const rim = TUBE + 2.5;
    const ringAt = (x) => Array.from({ length: 64 }, (_, i) => [x, Math.cos((i / 64) * 2 * Math.PI) * rim, Math.sin((i / 64) * 2 * Math.PI) * rim]);
    const eras = bands
        .map((b) => ({ ...b, clusters: clusters.filter((c) => c.era === b.era) }))
        .filter((e) => e.clusters.length)
        .map((e) => ({ ...e, ring: ringAt(e.from - 1), points: e.clusters.flatMap((c) => c.dots.map((d) => d.p)) }));
    const last = bands.at(-1);
    const end = ringAt(last.from + last.width + 1);
    const axis = [
        [bands[0].from - 3, 0, 0],
        [last.from + last.width + 3, 0, 0],
    ];
    return { clusters, dots, eras, end, axis, rim };
}

const el = (tag, props = {}, ...children) => {
    const node = Object.assign(document.createElement(tag), props);
    node.append(...children);
    return node;
};

export function mountCloud(root, pieces) {
    const world = layout(pieces);
    const stage = root.querySelector(".cloud");
    const canvas = stage.querySelector("canvas");
    const ctx = canvas.getContext("2d");
    const crumbs = stage.querySelector(".crumbs");
    const tools = stage.querySelector(".tools");
    const hint = stage.querySelector(".hint");
    const card = root.querySelector(".piece");
    const index = root.querySelector(".index");
    const reduced = matchMedia("(prefers-reduced-motion: reduce)");
    const colors = {};

    let width = 0;
    let height = 0;
    let dpr = 1;
    let focal = 1;
    const cam = { t: [0, 0, 0], d: 100, ...HOME };
    let homeD = 100;
    let hovered = null;
    let selected = null;
    let focus = { era: null, cluster: null };
    let frame = 0;
    let flight = null;
    let spin = null;

    const readColors = () => {
        const css = getComputedStyle(document.documentElement);
        colors.ink = getComputedStyle(document.body).color;
        colors.bg = getComputedStyle(document.body).backgroundColor;
        colors.muted = getComputedStyle(stage.querySelector(".probe")).color;
        colors.accent = getComputedStyle(stage).color;
        colors.mono = css.getPropertyValue("--font-mono");
        colors.body = css.getPropertyValue("--font-body");
        draw();
    };

    const rotate = (p, view = cam) => {
        let x = p[0] - view.t[0];
        let y = p[1] - view.t[1];
        let z = p[2] - view.t[2];
        const cy = Math.cos(view.yaw);
        const sy = Math.sin(view.yaw);
        [x, z] = [x * cy - z * sy, x * sy + z * cy];
        const cp = Math.cos(view.pitch);
        const sp = Math.sin(view.pitch);
        [y, z] = [y * cp - z * sp, y * sp + z * cp];
        return [x, y, z];
    };
    const unrotate = ([x, y, z]) => {
        const cp = Math.cos(cam.pitch);
        const sp = Math.sin(cam.pitch);
        [y, z] = [y * cp + z * sp, -y * sp + z * cp];
        const cy = Math.cos(cam.yaw);
        const sy = Math.sin(cam.yaw);
        [x, z] = [x * cy + z * sy, -x * sy + z * cy];
        return [x, y, z];
    };
    const project = (p) => {
        const [x, y, z] = rotate(p);
        const depth = z + cam.d;
        if (depth < 0.4) return null;
        const s = focal / depth;
        return { x: width / 2 + x * s, y: height / 2 - y * s, s, depth };
    };

    const centre = (points) => {
        const lo = [0, 1, 2].map((k) => Math.min(...points.map((p) => p[k])));
        const hi = [0, 1, 2].map((k) => Math.max(...points.map((p) => p[k])));
        return lo.map((v, k) => (v + hi[k]) / 2);
    };
    const fit = (points, pad, view = cam) => {
        const t = centre(points);
        let d = 4;
        for (const p of points) {
            const [x, y, z] = rotate(p, { ...view, t });
            d = Math.max(d, (Math.abs(x) * focal * pad) / (width / 2) - z, (Math.abs(y) * focal * pad) / (height / 2 - 40) - z);
        }
        return { t, d };
    };
    const everything = () => [...world.dots.map((d) => d.p), ...world.eras.flatMap((e) => e.ring), ...world.end];
    const clampD = (d) => clamp(d, 3, homeD * 1.6);
    const dotRadius = (d, q) => clamp((d.big ? 0.62 : 0.4) * q.s, 1.3, d.big ? 7 : 4.5);

    function resize() {
        const first = width === 0;
        dpr = Math.min(devicePixelRatio || 1, 2);
        width = stage.clientWidth;
        height = stage.clientHeight;
        canvas.width = Math.round(width * dpr);
        canvas.height = Math.round(height * dpr);
        focal = height * 1.3;
        const home = fit(everything(), 1.04, { ...cam, ...HOME });
        homeD = home.d;
        if (first) Object.assign(cam, home);
        draw();
    }

    const labels = [];
    const overlaps = (a) => labels.some((l) => a.x < l.x + l.w && a.x + a.w > l.x && a.y < l.y + l.h && a.y + a.h > l.y);
    const claim = (box) => {
        if (overlaps(box)) return false;
        labels.push(box);
        return true;
    };

    const nameText = (c) => c.name.toUpperCase();
    const nameBox = (c, q) => {
        ctx.font = `${NAME_FONT} ${colors.mono}`;
        ctx.letterSpacing = "0.06em";
        const w = ctx.measureText(nameText(c)).width + 8;
        ctx.letterSpacing = "0px";
        const bottom = q.y - Math.max(c.radius * q.s, 7) - 5;
        return { x: q.x - w / 2, y: bottom - 15, w, h: 15 };
    };

    function line(points, close) {
        ctx.beginPath();
        let open = false;
        for (const p of close ? [...points, points[0]] : points) {
            const q = project(p);
            if (!q) {
                open = false;
                continue;
            }
            if (open) ctx.lineTo(q.x, q.y);
            else ctx.moveTo(q.x, q.y);
            open = true;
        }
        ctx.stroke();
    }

    function draw() {
        if (!width || !colors.ink) return;
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, width, height);
        labels.length = 0;
        const frameBox = stage.getBoundingClientRect();
        for (const node of [crumbs, tools]) {
            const r = node.getBoundingClientRect();
            labels.push({ x: r.left - frameBox.left - 4, y: r.top - frameBox.top - 4, w: r.width + 8, h: r.height + 8 });
        }
        const zoom = homeD / cam.d;
        const quiet = focus.cluster ? 0.35 : 1;

        ctx.strokeStyle = colors.muted;
        ctx.lineWidth = 1;
        ctx.globalAlpha = 0.35 * quiet;
        line(world.axis);
        ctx.globalAlpha = 0.12 * quiet;
        for (const e of world.eras) line(e.ring, true);
        line(world.end, true);

        const eraAlpha = clamp((3.2 - zoom) / 1.6, 0, 1);
        ctx.font = `11px ${colors.mono}`;
        ctx.textAlign = "center";
        ctx.fillStyle = colors.muted;
        for (const e of world.eras) {
            const alpha = (focus.era === e.era.id ? 1 : eraAlpha) * quiet;
            if (!alpha) continue;
            const q = project([e.from + e.width / 2, world.rim + 1.5, 0]);
            if (!q) continue;
            const text = `${e.era.name.toUpperCase()} ${e.era.from}`;
            const tw = ctx.measureText(text).width;
            if (!claim({ x: q.x - tw / 2, y: q.y - 12, w: tw, h: 14 })) continue;
            ctx.globalAlpha = alpha * 0.9;
            ctx.fillText(text, q.x, q.y);
        }

        for (const d of world.dots) d.q = null;
        const shown = world.dots
            .map((d) => ({ d, q: project(d.p) }))
            .filter(({ q }) => q && q.x > -30 && q.x < width + 30 && q.y > -30 && q.y < height + 30)
            .sort((a, b) => b.q.depth - a.q.depth);
        const depths = shown.map(({ q }) => q.depth);
        const near = Math.min(...depths);
        const far = Math.max(...depths);
        for (const { d, q } of shown) {
            d.q = q;
            const fade = far > near ? (far - q.depth) / (far - near) : 1;
            const dim = focus.cluster ? (d.cluster === focus.cluster ? 1 : 0.1) : focus.era && d.cluster.era.id !== focus.era ? 0.25 : 1;
            const r = dotRadius(d, q);
            const hot = d === hovered || d === selected;
            ctx.globalAlpha = hot ? 1 : dim * (0.3 + 0.7 * fade);
            ctx.fillStyle = hot ? colors.accent : colors.ink;
            ctx.beginPath();
            ctx.arc(q.x, q.y, r, 0, Math.PI * 2);
            ctx.fill();
            if (hot) {
                ctx.strokeStyle = colors.accent;
                ctx.lineWidth = 1;
                ctx.beginPath();
                ctx.arc(q.x, q.y, r + 4, 0, Math.PI * 2);
                ctx.stroke();
            }
        }
        ctx.globalAlpha = 1;

        const centres = world.clusters.map((c) => ({ c, q: project(c.p) })).filter(({ q }) => q);
        const titled = focus.cluster;
        const head = titled && project(titled.p);
        const headBox = head && nameBox(titled, head);
        if (headBox) labels.push(headBox);

        ctx.lineJoin = "round";
        if (titled) {
            ctx.textAlign = "left";
            ctx.font = `${TITLE_FONT} ${colors.body}`;
            const own = shown
                .filter(({ d }) => d.cluster === titled)
                .sort((a, b) => (b.d === selected) - (a.d === selected) || a.q.depth - b.q.depth);
            const placed = [];
            for (const { d, q } of own) {
                if (q.x > width - 20 || q.y < 20 || q.y > height - 10) continue;
                const x = q.x + dotRadius(d, q) + 5;
                const text = d.piece.title.length > 34 ? `${d.piece.title.slice(0, 33)}…` : d.piece.title;
                if (claim({ x, y: q.y - 6, w: ctx.measureText(text).width, h: 12 })) placed.push([text, x, q.y + 4]);
            }
            ctx.lineWidth = 4;
            ctx.strokeStyle = colors.bg;
            ctx.fillStyle = colors.muted;
            for (const [text, x, y] of placed) {
                ctx.strokeText(text, x, y);
                ctx.fillText(text, x, y);
            }
        }

        if (zoom > 1.5 || titled) {
            ctx.textAlign = "center";
            for (const { c, q } of centres.sort((a, b) => a.q.depth - b.q.depth)) {
                const box = c === titled ? headBox : nameBox(c, q);
                if (box.x + box.w < 0 || box.x > width || box.y < 6 || box.y + box.h > height - 2) continue;
                if (c !== titled && !claim(box)) continue;
                ctx.font = `${NAME_FONT} ${colors.mono}`;
                ctx.letterSpacing = "0.06em";
                const baseline = box.y + 11.5;
                if (c === titled) {
                    ctx.globalAlpha = 1;
                    ctx.fillStyle = colors.bg;
                    ctx.fillRect(box.x, box.y, box.w, box.h);
                    ctx.globalAlpha = 0.5;
                    ctx.strokeStyle = colors.ink;
                    ctx.lineWidth = 1;
                    ctx.strokeRect(box.x + 0.5, box.y + 0.5, box.w - 1, box.h - 1);
                    ctx.globalAlpha = 1;
                } else {
                    ctx.globalAlpha = focus.cluster ? 0.2 : 0.9;
                    ctx.lineWidth = 4;
                    ctx.strokeStyle = colors.bg;
                    if (!focus.cluster) ctx.strokeText(nameText(c), q.x, baseline);
                }
                ctx.fillStyle = colors.ink;
                ctx.fillText(nameText(c), q.x, baseline);
            }
            ctx.letterSpacing = "0px";
            ctx.globalAlpha = 1;
        }
    }

    const schedule = () => {
        if (!frame) frame = requestAnimationFrame(tick);
    };
    const ease = (t) => (t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2);

    function tick() {
        const time = performance.now();
        frame = 0;
        if (flight) {
            const t = Math.min(1, (time - flight.start) / flight.duration);
            const e = ease(t);
            cam.t = flight.from.t.map((v, k) => v + (flight.to.t[k] - v) * e);
            cam.d = Math.exp(Math.log(flight.from.d) + (Math.log(flight.to.d) - Math.log(flight.from.d)) * e);
            if (flight.to.yaw !== undefined) cam.yaw = flight.from.yaw + (flight.to.yaw - flight.from.yaw) * e;
            if (t >= 1) flight = null;
        }
        if (spin) {
            const dt = Math.min(48, time - (spin.last ?? time));
            spin.last = time;
            cam.yaw += spin.yaw * dt;
            cam.pitch = clamp(cam.pitch + spin.pitch * dt, -PITCH, PITCH);
            const decay = 0.94 ** (dt / 16);
            spin.yaw *= decay;
            spin.pitch *= decay;
            if (Math.abs(spin.yaw) + Math.abs(spin.pitch) < 0.00002) spin = null;
        }
        draw();
        if (flight || spin) schedule();
    }

    function flyTo(target) {
        const to = { t: target.t, d: clampD(target.d) };
        if (reduced.matches) {
            Object.assign(cam, to);
            draw();
        } else {
            flight = { from: { t: cam.t, d: cam.d, yaw: cam.yaw }, to, start: performance.now(), duration: 700 };
            schedule();
        }
        updateNav();
    }

    function zoomTo(target) {
        if (!target) {
            focus = { era: null, cluster: null };
            select(null);
            flyTo(fit(everything(), 1.04));
            return;
        }
        if (target.era) {
            const e = world.eras.find((x) => x.era.id === target.era);
            focus = { era: e.era.id, cluster: null };
            select(null);
            flyTo(fit(e.points, 1.25));
            return;
        }
        const c = target.cluster;
        focus = { era: c.era.id, cluster: c };
        select(selected?.cluster === c ? selected : c.dots[0]);
        const view = fit(
            c.dots.map((d) => d.p),
            1.5,
        );
        flyTo({ t: c.p, d: Math.min(view.d, (focal * SPACING) / 58) });
    }

    function zoomAt(sx, sy, factor) {
        const d = clampD(cam.d / factor);
        const scale = 1 - d / cam.d;
        const under = unrotate([((sx - width / 2) * cam.d) / focal, (-(sy - height / 2) * cam.d) / focal, 0]);
        cam.t = cam.t.map((v, k) => v + under[k] * scale);
        cam.d = d;
        flight = null;
        hint.hidden = true;
        schedule();
    }

    function select(d) {
        selected = d;
        const p = d?.piece;
        const rows = [
            ["piece", p?.title],
            ["composer", p?.composer],
            ["year", p && `${yearText(p)} · ${d.cluster.era.name}`],
            ["form", p && [p.form, p.scoring].filter(Boolean).join(" · ")],
            ["recorded", p?.album],
        ];
        card.classList.toggle("empty", !d);
        card.querySelector("dl").replaceChildren(...rows.flatMap(([k, v]) => [el("dt", { textContent: k }), el("dd", { textContent: v ?? "", title: v ?? "" })]));
        updateNav();
        draw();
    }

    function up() {
        if (focus.cluster) zoomTo({ era: focus.era });
        else if (focus.era) zoomTo(null);
    }

    const item = (label, note, go, state) => {
        const b = el("button", { type: "button" }, label);
        if (note) b.append(el("small", { textContent: note }));
        if (state) b.setAttribute(...state);
        b.addEventListener("click", go);
        return el("li", {}, b);
    };

    function updateNav() {
        const era = focus.era && world.eras.find((e) => e.era.id === focus.era);
        const trail = [["All", () => zoomTo(null)]];
        if (era) trail.push([era.era.name, () => zoomTo({ era: era.era.id })]);
        if (focus.cluster) trail.push([focus.cluster.name, () => zoomTo({ cluster: focus.cluster })]);
        crumbs.replaceChildren(...trail.map(([label, go], i) => item(label, null, go, i === trail.length - 1 && ["aria-current", "true"])));

        const label = index.querySelector("p");
        const list = index.querySelector("ul");
        if (focus.cluster) {
            label.textContent = `Pieces by ${focus.cluster.name}`;
            list.replaceChildren(...focus.cluster.dots.map((d) => item(d.piece.title, null, () => select(d), ["aria-pressed", String(d === selected)])));
        } else if (era) {
            label.textContent = `${era.era.name} composers`;
            list.replaceChildren(...era.clusters.toSorted((a, b) => a.name.localeCompare(b.name)).map((c) => item(c.name, null, () => zoomTo({ cluster: c }))));
        } else {
            label.textContent = "Eras";
            list.replaceChildren(...world.eras.map((e) => item(e.era.name, null, () => zoomTo({ era: e.era.id }))));
        }
    }

    const hit = (sx, sy) => {
        let best = null;
        let bestD = 12 ** 2;
        for (const d of world.dots) {
            if (!d.q) continue;
            const dist = (d.q.x - sx) ** 2 + (d.q.y - sy) ** 2;
            if (dist < bestD || (best && dist === bestD && d.q.depth < best.q.depth)) {
                bestD = dist;
                best = d;
            }
        }
        return best;
    };

    function choose(d) {
        if (d === selected) up();
        else if (focus.cluster === d.cluster) select(d);
        else if (homeD / cam.d < 1.5 && focus.era !== d.cluster.era.id) {
            zoomTo({ era: d.cluster.era.id });
        } else {
            zoomTo({ cluster: d.cluster });
        }
    }

    const local = (event) => {
        const box = canvas.getBoundingClientRect();
        return [event.clientX - box.left, event.clientY - box.top];
    };

    stage.addEventListener(
        "wheel",
        (event) => {
            if (!event.ctrlKey && !event.metaKey) return;
            event.preventDefault();
            const [sx, sy] = local(event);
            zoomAt(sx, sy, Math.exp(-event.deltaY * 0.01));
        },
        { passive: false },
    );

    const pointers = new Map();
    let drag = null;
    stage.addEventListener("pointerdown", (event) => {
        if (event.target.closest("button, input")) return;
        pointers.set(event.pointerId, local(event));
        drag = { moved: 0, pinch: undefined, velocity: { yaw: 0, pitch: 0 }, at: event.timeStamp };
        spin = null;
    });
    stage.addEventListener("pointermove", (event) => {
        const [sx, sy] = local(event);
        if (!pointers.has(event.pointerId)) {
            const d = hit(sx, sy);
            if (d !== hovered) {
                hovered = d;
                stage.classList.toggle("over", Boolean(d));
                canvas.title = d ? `${d.piece.title} · ${d.piece.composer}` : "";
                schedule();
            }
            return;
        }
        const [px, py] = pointers.get(event.pointerId);
        pointers.set(event.pointerId, [sx, sy]);
        if (pointers.size === 2) {
            const [a, b] = [...pointers.values()];
            const span = Math.hypot(a[0] - b[0], a[1] - b[1]);
            if (drag.pinch) zoomAt((a[0] + b[0]) / 2, (a[1] + b[1]) / 2, span / drag.pinch);
            drag.pinch = span;
            drag.moved += 10;
            return;
        }
        drag.moved += Math.abs(sx - px) + Math.abs(sy - py);
        if (drag.moved <= 4) return;
        if (!stage.hasPointerCapture(event.pointerId)) stage.setPointerCapture(event.pointerId);
        stage.classList.add("dragging");
        hint.hidden = true;
        const dyaw = (sx - px) * 0.007;
        const dpitch = (sy - py) * 0.007;
        cam.yaw += dyaw;
        cam.pitch = clamp(cam.pitch + dpitch, -PITCH, PITCH);
        const dt = Math.max(8, event.timeStamp - drag.at);
        drag.velocity = { yaw: dyaw / dt, pitch: dpitch / dt };
        drag.at = event.timeStamp;
        schedule();
    });
    const release = (event) => {
        if (!pointers.has(event.pointerId)) return;
        pointers.delete(event.pointerId);
        stage.classList.remove("dragging");
        if (event.type === "pointerup" && drag && pointers.size === 0) {
            if (drag.moved > 4) {
                if (!reduced.matches && event.timeStamp - drag.at < 80) {
                    spin = { ...drag.velocity };
                    schedule();
                }
            } else {
                const [sx, sy] = local(event);
                const d = hit(sx, sy);
                if (d) choose(d);
            }
        }
        if (pointers.size === 0) drag = null;
    };
    stage.addEventListener("pointerup", release);
    stage.addEventListener("pointercancel", release);
    stage.addEventListener("pointerleave", () => {
        if (hovered && !drag) {
            hovered = null;
            stage.classList.remove("over");
            schedule();
        }
    });
    stage.addEventListener("dblclick", (event) => {
        const [sx, sy] = local(event);
        zoomAt(sx, sy, 2);
    });

    stage.addEventListener("keydown", (event) => {
        if (event.target !== stage) return;
        const turn = (yaw, pitch) => () => {
            cam.yaw += yaw;
            cam.pitch = clamp(cam.pitch + pitch, -PITCH, PITCH);
            schedule();
        };
        const keys = {
            "+": () => zoomAt(width / 2, height / 2, 1.5),
            "=": () => zoomAt(width / 2, height / 2, 1.5),
            "-": () => zoomAt(width / 2, height / 2, 1 / 1.5),
            ArrowLeft: turn(-0.15, 0),
            ArrowRight: turn(0.15, 0),
            ArrowUp: turn(0, -0.15),
            ArrowDown: turn(0, 0.15),
            Escape: up,
        };
        if (keys[event.key]) {
            event.preventDefault();
            keys[event.key]();
        }
    });

    tools.querySelector(".zoom-in").addEventListener("click", () => zoomAt(width / 2, height / 2, 1.6));
    tools.querySelector(".zoom-out").addEventListener("click", () => zoomAt(width / 2, height / 2, 1 / 1.6));

    const find = tools.querySelector("input");
    const clusterLabel = (c) => `${c.name} (${c.dots.length})`;
    const dotLabel = (d) => `${d.piece.title} · ${d.piece.composer}`;
    tools.querySelector("datalist").append(...world.clusters.map((c) => new Option(clusterLabel(c))), ...world.dots.map((d) => new Option(dotLabel(d))));
    find.addEventListener("change", () => {
        const value = find.value.trim();
        const dot = world.dots.find((d) => dotLabel(d) === value);
        const cluster = world.clusters.find((c) => clusterLabel(c) === value || c.name === value);
        if (dot) {
            zoomTo({ cluster: dot.cluster });
            select(dot);
        } else if (cluster) {
            zoomTo({ cluster });
        }
    });

    matchMedia("(prefers-color-scheme: dark)").addEventListener("change", readColors);
    select(null);
    readColors();
    resize();
    new ResizeObserver(resize).observe(stage);

    const deep = new URLSearchParams(location.hash.slice(1));
    const deepCluster = world.clusters.find((c) => c.name === deep.get("c"));
    if (world.eras.some((e) => e.era.id === deep.get("era"))) zoomTo({ era: deep.get("era") });
    if (deepCluster) {
        zoomTo({ cluster: deepCluster });
        const dot = deepCluster.dots.find((d) => d.piece.title === deep.get("p"));
        if (dot) select(dot);
    }
    if (!deepCluster && !deep.get("era") && !reduced.matches) {
        new IntersectionObserver(
            (entries, observer) => {
                if (!entries[0].isIntersecting) return;
                observer.disconnect();
                cam.yaw = HOME.yaw - 1.2;
                flight = { from: { t: cam.t, d: cam.d, yaw: cam.yaw }, to: { t: cam.t, d: cam.d, yaw: HOME.yaw }, start: performance.now(), duration: 2200 };
                schedule();
            },
            { threshold: 0.5 },
        ).observe(stage);
    }
}
