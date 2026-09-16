// Bit->segment mapping (bit0=a, bit1=b, ... bit6=g) is a best-effort
// placeholder pending schematic verification (see
// docs/kim1-memory-map.md) — easy to correct here in one place without
// touching the Go emulation core. It happens to match the real monitor
// ROM's own hex segment table byte-for-byte, so it's very likely right.
const SEGMENTS = ["a", "b", "c", "d", "e", "f", "g"];

// Electrical scan-matrix position (row, col) for each key legend, matching
// kim1.Keypad's 3x7 matrix — verified empirically against the real
// monitor ROM (pressing each (row,col) and reading what it entered on the
// display), including AD/PC, which were initially swapped: AD is the key
// that, after DA, correctly routes further digit entry back into the
// address field.
const ELECTRICAL = {
  "0": [0, 6], "1": [0, 5], "2": [0, 4], "3": [0, 3],
  "4": [0, 2], "5": [0, 1], "6": [0, 0], "7": [1, 6],
  "8": [1, 5], "9": [1, 4], "A": [1, 3], "B": [1, 2],
  "C": [1, 1], "D": [1, 0], "E": [2, 6], "F": [2, 5],
  "AD": [2, 4], "DA": [2, 3], "+": [2, 2], "GO": [2, 1], "PC": [2, 0],
};

// Visual layout: 4 columns x 6 rows, matching the physical arrangement on
// a real KIM-1 board (photographed layout, not the electrical matrix
// above). RS and ST sit in this same top row on real hardware but are
// wired directly to CPU RESET/NMI rather than being scanned through the
// keypad matrix; SST is a physical slide switch, not a momentary key —
// shown for visual authenticity but not yet wired to anything.
const VISUAL_LAYOUT = [
  ["GO", "ST", "RS", "SST"],
  ["AD", "DA", "PC", "+"],
  ["C", "D", "E", "F"],
  ["8", "9", "A", "B"],
  ["4", "5", "6", "7"],
  ["0", "1", "2", "3"],
];

const displayEl = document.getElementById("display");
const keypadEl = document.getElementById("keypad");

const digitEls = [];
for (let i = 0; i < 6; i++) {
  const d = document.createElement("div");
  // Digits 0-3 are the address field, 4-5 are data -- a real KIM-1
  // board has a visible gap between the two groups on its display.
  d.className = i === 4 ? "digit digit-group-gap" : "digit";
  const segs = {};
  for (const s of SEGMENTS) {
    const el = document.createElement("div");
    el.className = "seg seg-" + s;
    d.appendChild(el);
    segs[s] = el;
  }
  displayEl.appendChild(d);
  digitEls.push(segs);
}

function setDigit(index, bits) {
  const segs = digitEls[index];
  for (let b = 0; b < SEGMENTS.length; b++) {
    const on = (bits & (1 << b)) !== 0;
    segs[SEGMENTS[b]].classList.toggle("on", on);
  }
}

let socket = null;
let sstBtn = null;

const ttySwitchBtn = document.getElementById("tty-switch");
const ttyOutEl = document.getElementById("tty-out");
const ttyForm = document.getElementById("tty-form");
const ttyInEl = document.getElementById("tty-in");

ttySwitchBtn.addEventListener("click", () => {
  const on = !ttySwitchBtn.classList.contains("on");
  ttySwitchBtn.classList.toggle("on", on);
  sendMsg({ type: "ttyselect", down: on });
});

// Real teletypes echo what you type locally (the keyboard is
// mechanically linked to the printer, independent of what's actually
// received), so without that there'd be no visual trace of your own
// input at all. The server tags each transcript run as sent (typed by
// you) or received (decoded from the CPU's TTY output); render them in
// different colors so the two are distinguishable at a glance.
function renderTTY(runs) {
  ttyOutEl.textContent = "";
  for (const run of runs) {
    const span = document.createElement("span");
    span.className = run.sent ? "tty-sent" : "tty-recv";
    span.textContent = run.text;
    ttyOutEl.appendChild(span);
  }
  ttyOutEl.scrollTop = ttyOutEl.scrollHeight;
}

ttyForm.addEventListener("submit", (e) => {
  e.preventDefault();
  // A real teletype's RETURN key sends just CR ($0D); the monitor's own
  // GETCH/OUTCH echo supplies the LF back, which is why decoded output
  // shows up as "\r\n".
  sendMsg({ type: "ttysend", text: ttyInEl.value + "\r" });
  ttyInEl.value = "";
});

function sendMsg(msg) {
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(msg));
  }
}

for (const rowLabels of VISUAL_LAYOUT) {
  for (const label of rowLabels) {
    const btn = document.createElement("button");
    btn.textContent = label;

    if (label === "SST") {
      btn.className = "key switch";
      btn.title = "Single-step: NMI after every instruction outside ROM. Needs the NMI vector at $17FA/$17FB set up first (see README).";
      btn.addEventListener("click", () => {
        const on = !btn.classList.contains("on");
        btn.classList.toggle("on", on);
        sendMsg({ type: "sst", down: on });
      });
      sstBtn = btn;
    } else if (label === "RS") {
      btn.className = "key ctrl";
      btn.title = "Reset (direct to CPU RESET, not part of the keypad matrix)";
      btn.addEventListener("click", () => sendMsg({ type: "reset" }));
    } else if (label === "ST") {
      btn.className = "key ctrl";
      btn.title = "Single-step (direct to CPU NMI, not part of the keypad matrix)";
      btn.addEventListener("click", () => sendMsg({ type: "nmi" }));
    } else {
      btn.className = "key";
      const [row, col] = ELECTRICAL[label];
      const press = (down) => (e) => {
        e.preventDefault();
        btn.classList.toggle("pressed", down);
        sendMsg({ type: "key", row, col, down });
      };
      btn.addEventListener("pointerdown", press(true));
      btn.addEventListener("pointerup", press(false));
      btn.addEventListener("pointerleave", press(false));
    }

    keypadEl.appendChild(btn);
  }
}

const connstate = document.getElementById("connstate");
const hex = (v, digits) => v.toString(16).toUpperCase().padStart(digits, "0");

// I/O port LED panel: one row per RIOT port (App/Kbd x A/B), 8 LEDs each
// (bit7 on the left .. bit0 on the right), lit to the pin's current
// electrical level (output bits as driven, input bits as last sampled —
// same as what a logic probe would read). Blue marks an input pin (DDR
// bit 0), red an output pin (DDR bit 1) -- color, not just lit/unlit, so
// the two are distinguishable at a glance (see .ioled in style.css).
//
// The App RIOT's pins (the real KIM-1's user application connector) also
// get a row of switches beneath, standing in for hobbyist-wired toggle
// switches -- there's nothing else driving those pins. The Kbd RIOT's
// pins are all already dedicated to the keypad, display, and TTY, so
// they get none.
const IO_PORTS = [
  { key: "appPA", label: "App PA", switchable: true, port: "A" },
  { key: "appPB", label: "App PB", switchable: true, port: "B" },
  { key: "kbdPA", label: "Kbd PA", switchable: false },
  { key: "kbdPB", label: "Kbd PB", switchable: false },
];

const ioportsGrid = document.getElementById("ioports-grid");
const ioLedEls = {};
const ioSwitchEls = {};

function buildBitRow(extraClass, makeEl) {
  const row = document.createElement("div");
  row.className = extraClass ? "ioport-row " + extraClass : "ioport-row";

  const labelEl = document.createElement("span");
  labelEl.className = "ioport-label";
  row.appendChild(labelEl);

  const wrap = document.createElement("div");
  wrap.className = "ioport-leds";
  const bitEls = [];
  for (let bit = 7; bit >= 0; bit--) {
    const el = makeEl(bit);
    wrap.appendChild(el);
    bitEls[bit] = el;
  }
  row.appendChild(wrap);
  ioportsGrid.appendChild(row);
  return { row, labelEl, bitEls };
}

for (const { key, label, switchable, port } of IO_PORTS) {
  const leds = buildBitRow(null, (bit) => {
    const led = document.createElement("div");
    led.className = "ioled";
    led.title = "bit " + bit;
    return led;
  });
  leds.labelEl.textContent = label;
  ioLedEls[key] = leds.bitEls;

  if (switchable) {
    const switches = buildBitRow("ioport-switches", (bit) => {
      const btn = document.createElement("button");
      btn.className = "ioswitch";
      btn.title = "Set App RIOT Port " + port + " bit " + bit + " input level (only takes effect while that pin is configured as an input)";
      btn.addEventListener("click", () => {
        const on = !btn.classList.contains("on");
        btn.classList.toggle("on", on);
        sendMsg({ type: "appswitch", port, bit, down: on });
      });
      return btn;
    });
    ioSwitchEls[key] = switches.bitEls;
  }
}

function updatePort(key, port) {
  const bitEls = ioLedEls[key];
  for (let bit = 0; bit < 8; bit++) {
    const on = (port.value & (1 << bit)) !== 0;
    const isOutput = (port.ddr & (1 << bit)) !== 0;
    const led = bitEls[bit];
    led.classList.toggle("on", on);
    led.classList.toggle("output", isOutput);
  }
}

function updateSwitches(key, value) {
  const bitEls = ioSwitchEls[key];
  if (!bitEls) return;
  for (let bit = 0; bit < 8; bit++) {
    bitEls[bit].classList.toggle("on", (value & (1 << bit)) !== 0);
  }
}

function connect() {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  socket = new WebSocket(proto + "//" + location.host + "/ws");

  socket.onopen = () => { connstate.textContent = "connected"; };
  socket.onclose = () => {
    connstate.textContent = "disconnected — retrying…";
    setTimeout(connect, 1000);
  };
  socket.onerror = () => socket.close();

  socket.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type !== "state") return;

    for (let i = 0; i < 6; i++) setDigit(i, msg.digits[i]);

    document.getElementById("reg-a").textContent = hex(msg.a, 2);
    document.getElementById("reg-x").textContent = hex(msg.x, 2);
    document.getElementById("reg-y").textContent = hex(msg.y, 2);
    document.getElementById("reg-sp").textContent = hex(msg.sp, 2);
    document.getElementById("reg-pc").textContent = hex(msg.pc, 4);
    document.getElementById("reg-p").textContent = hex(msg.p, 2);
    document.getElementById("reg-cycles").textContent = msg.cycles;
    document.getElementById("instr").textContent = msg.instr;

    updatePort("appPA", msg.appPA);
    updatePort("appPB", msg.appPB);
    updatePort("kbdPA", msg.kbdPA);
    updatePort("kbdPB", msg.kbdPB);
    updateSwitches("appPA", msg.appSwitchA);
    updateSwitches("appPB", msg.appSwitchB);

    if (sstBtn) sstBtn.classList.toggle("on", msg.sst);
    ttySwitchBtn.classList.toggle("on", msg.ttySelect);
    renderTTY(msg.ttyLog || []);

    const flagNames = ["C", "Z", "I", "D", "B", "-", "V", "N"];
    const flags = flagNames
      .map((name, i) => ((msg.p & (1 << i)) ? name : "."))
      .reverse()
      .join(" ");
    document.getElementById("flags").textContent = flags;

    const banner = document.getElementById("haltbanner");
    if (msg.halted) {
      banner.textContent = "CPU halted: " + msg.error + " — press RS to reset";
      banner.hidden = false;
    } else {
      banner.hidden = true;
    }
  };
}

connect();
