// 把浏览器 KeyboardEvent.code/key 映射为 AutoHotkey v2 的键名。

// 需要特殊映射的按键（浏览器 code -> AHK 名称）。
const SPECIAL = {
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  Escape: 'Esc',
  Enter: 'Enter',
  NumpadEnter: 'NumpadEnter',
  Space: 'Space',
  Tab: 'Tab',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Insert: 'Insert',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  CapsLock: 'CapsLock',
  PrintScreen: 'PrintScreen',
  ScrollLock: 'ScrollLock',
  Pause: 'Pause',
  ContextMenu: 'AppsKey',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Backslash: '\\',
  Semicolon: ';',
  Quote: "'",
  Comma: ',',
  Period: '.',
  Slash: '/',
  Backquote: '`',
  NumpadAdd: 'NumpadAdd',
  NumpadSubtract: 'NumpadSub',
  NumpadMultiply: 'NumpadMult',
  NumpadDivide: 'NumpadDiv',
  NumpadDecimal: 'NumpadDot',
  NumpadComma: 'NumpadDot',
};

// 修饰键的 code 集合，录制时需要忽略它们本身。
const MODIFIER_CODES = new Set([
  'ControlLeft', 'ControlRight',
  'ShiftLeft', 'ShiftRight',
  'AltLeft', 'AltRight',
  'MetaLeft', 'MetaRight',
]);

// 判断某个 code 是否为修饰键。
export function isModifierCode(code) {
  return MODIFIER_CODES.has(code);
}

// 返回非修饰键对应的 AHK 键名；无法识别时返回 null。
export function keyFromEvent(e) {
  const code = e.code;
  if (MODIFIER_CODES.has(code)) return null;
  if (/^Key[A-Z]$/.test(code)) return code.slice(3).toLowerCase(); // KeyA -> a
  if (/^Digit[0-9]$/.test(code)) return code.slice(5);            // Digit1 -> 1
  if (/^Numpad[0-9]$/.test(code)) return 'Numpad' + code.slice(6); // Numpad1 -> Numpad1
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code)) return code;          // F1..F24
  if (SPECIAL[code]) return SPECIAL[code];
  return null;
}

// 从事件中提取组合键修饰符状态。
export function modsFromEvent(e) {
  return {
    ctrl: e.ctrlKey,
    alt: e.altKey,
    shift: e.shiftKey,
    win: e.metaKey,
  };
}

// 生成便于显示的键名（单字符大写，其余原样）。
export function prettyKey(name) {
  if (!name) return '';
  if (name.length === 1) return name.toUpperCase();
  return name;
}
