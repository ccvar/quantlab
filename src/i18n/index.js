import { enUS } from "./en-US.js";
import { zhCN } from "./zh-CN.js";

export const DEFAULT_LOCALE = "zh-CN";

export const locales = {
  "zh-CN": zhCN,
  "en-US": enUS,
};

export function resolveLocale(value) {
  if (value && locales[value]) return value;
  return DEFAULT_LOCALE;
}

export function formatMessage(message, values = {}) {
  return String(message || "").replace(/\{(\w+)\}/g, (_, key) =>
    Object.prototype.hasOwnProperty.call(values, key) ? String(values[key]) : `{${key}}`,
  );
}

export function makeTranslator(locale) {
  const resolvedLocale = resolveLocale(locale);
  const dictionary = locales[resolvedLocale] || locales[DEFAULT_LOCALE];
  const fallback = locales["en-US"];

  return function translate(key, fallbackText = key, values = {}) {
    const parts = String(key).split(".");
    let current = dictionary;
    let fallbackCurrent = fallback;
    for (const part of parts) {
      current = current && Object.prototype.hasOwnProperty.call(current, part) ? current[part] : undefined;
      fallbackCurrent =
        fallbackCurrent && Object.prototype.hasOwnProperty.call(fallbackCurrent, part)
          ? fallbackCurrent[part]
          : undefined;
    }
    const message = typeof current === "string" ? current : typeof fallbackCurrent === "string" ? fallbackCurrent : fallbackText;
    return formatMessage(message, values);
  };
}

export function choiceLabel(t, value) {
  return t(`choices.${value}`, value);
}
