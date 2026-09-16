// Bit->segment mapping (bit0=a, bit1=b, ... bit6=g) and the keypad
// row/column layout below are best-effort placeholders pending schematic
// verification (see docs/kim1-memory-map.md) — easy to correct here in
// one place without touching the Go emulation core.
const SEGMENTS = ["a", "b", "c", "d", "e", "f", "g"];

// 3 rows x 7 columns, matching kim1.Keypad's matrix. RS and ST are real
// KIM-1 controls wired directly to CPU RESET/NMI, not part of this matrix.
const KEY_LAYOUT = [
  ["0", "1", "2", "3", "4", "5", "6"],
  ["7", "8", "9", "A", "B", "C", "D"],
  ["E", "F", "AD", "DA", "+", "GO", "PC"],
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

function sendMsg(msg) {
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify(msg));
  }
}

for (let row = 0; row < KEY_LAYOUT.length; row++) {
  for (let col = 0; col < KEY_LAYOUT[row].length; col++) {
    const label = KEY_LAYOUT[row][col];
    const btn = document.createElement("button");
    btn.className = "key";
    btn.textContent = label;
    const press = (down) => (e) => {
      e.preventDefault();
      btn.classList.toggle("pressed", down);
      sendMsg({ type: "key", row, col, down });
    };
    btn.addEventListener("pointerdown", press(true));
    btn.addEventListener("pointerup", press(false));
    btn.addEventListener("pointerleave", press(false));
    keypadEl.appendChild(btn);
  }
}

document.getElementById("btn-rs").addEventListener("click", () => sendMsg({ type: "reset" }));
document.getElementById("btn-st").addEventListener("click", () => sendMsg({ type: "nmi" }));

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

    const flagNames = ["C", "Z", "I", "D", "B", "-", "V", "N"];
    const flags = flagNames
      .map((name, i) => ((msg.p & (1 << i)) ? name : "."))
      .reverse()
      .join(" ");
    document.getElementById("flags").textContent = flags;
  };
}

connect();
