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
  d.className = "digit";
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
    document.getElementById("instr").textContent = hex(msg.pc, 4) + ":  " + msg.instr;

    if (sstBtn) sstBtn.classList.toggle("on", msg.sst);

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
