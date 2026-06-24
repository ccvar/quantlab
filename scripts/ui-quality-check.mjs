#!/usr/bin/env node
import fs from "node:fs/promises";
import path from "node:path";
import { chromium } from "playwright";

const baseURL = process.env.CCVAR_UI_QUALITY_URL || "http://127.0.0.1:8787";
const outDir = process.env.CCVAR_UI_QUALITY_OUT_DIR || path.resolve("dist/ui-quality");
const localeInput = process.env.CCVAR_UI_QUALITY_LOCALES || process.env.CCVAR_UI_QUALITY_LOCALE || "zh-CN,en-US";
const locales = localeInput.split(",").map((value) => value.trim()).filter(Boolean);

const viewports = [
  { name: "desktop", width: 1366, height: 900 },
  { name: "tablet", width: 1024, height: 768 },
  { name: "breakpoint-901", width: 901, height: 900 },
  { name: "breakpoint-761", width: 761, height: 900 },
  { name: "tablet-portrait", width: 768, height: 1024 },
  { name: "mobile", width: 390, height: 844 },
];
const themes = ["dark", "light"];
const nativeScrollTargets = [
  ".run-list",
  ".controls-panel",
  ".indicator-strip",
  ".backtest-table-wrap",
  ".backtest-history-table",
  ".table-scroll",
  ".analytics-table-wrap",
  ".paper-table-wrap",
  ".autopilot-steps-view",
  ".event-log",
  ".modal-backdrop",
  ".credential-modal-grid",
  ".credential-form",
  ".credential-list",
  ".strategy-profile-body",
  ".ai-config-body",
  ".paper-reset-body",
  ".live-guard-grid",
  ".live-guard-grid > section",
  ".verdict-panel",
  ".audit-rows",
  ".workspace-tabs",
  ".bottom-tabs",
].join(",");
const scrollContainers = `${nativeScrollTargets},.wide-table-scroll`;

function assert(condition, message, details = undefined) {
  if (condition) return;
  const suffix = details ? `\n${JSON.stringify(details, null, 2)}` : "";
  throw new Error(`${message}${suffix}`);
}

async function newPage(browser, theme, viewport, locale) {
  const page = await browser.newPage({
    viewport: { width: viewport.width, height: viewport.height },
    deviceScaleFactor: 1,
  });
  const errors = [];
  page.on("console", (message) => {
    if (["error", "warning"].includes(message.type())) {
      errors.push({ type: message.type(), text: message.text().slice(0, 400) });
    }
  });
  page.on("pageerror", (error) => {
    errors.push({ type: "pageerror", text: String(error).slice(0, 400) });
  });
  await page.addInitScript(({ theme: nextTheme, locale: nextLocale }) => {
    localStorage.setItem("ccvar.theme", nextTheme);
    localStorage.setItem("ccvar.locale", nextLocale);
  }, { theme, locale });
  await page.goto(baseURL, { waitUntil: "networkidle" });
  await page.waitForTimeout(450);
  return { page, errors };
}

async function screenshot(page, name) {
  const filePath = path.join(outDir, `${name}.png`);
  await fs.mkdir(path.dirname(filePath), { recursive: true });
  await page.screenshot({ path: filePath, fullPage: false });
  return filePath;
}

async function layoutReport(page) {
  return page.evaluate((scrollSelector) => {
    const viewportWidth = window.innerWidth;
    const visible = Array.from(document.querySelectorAll("body *")).filter((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      return styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1;
    });
    const overflow = visible.map((element) => {
      const rect = element.getBoundingClientRect();
      const amount = Math.max(0, -rect.left, rect.right - viewportWidth);
      if (amount <= 2) return null;
      const scrollParent = element.closest(scrollSelector);
      const parentRect = scrollParent?.getBoundingClientRect();
      const contained = Boolean(scrollParent && parentRect.left >= -2 && parentRect.right <= viewportWidth + 2);
      const className = typeof element.className === "string" ? element.className : "";
      return {
        tag: element.tagName,
        className: className.slice(0, 120),
        text: (element.textContent || "").replace(/\s+/g, " ").trim().slice(0, 80),
        amount: Math.round(amount),
        contained,
        scrollParent: scrollParent ? String(scrollParent.className || scrollParent.tagName).slice(0, 100) : "",
      };
    }).filter(Boolean);
    const scrollbars = Array.from(document.querySelectorAll(scrollSelector)).map((element) => {
      const styles = getComputedStyle(element, "::-webkit-scrollbar");
      const rect = element.getBoundingClientRect();
      return {
        className: String(element.className || element.tagName),
        width: styles.width,
        height: styles.height,
        rect: { width: Math.round(rect.width), height: Math.round(rect.height) },
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
        scrollHeight: element.scrollHeight,
        clientHeight: element.clientHeight,
      };
    });
    return {
      pageScrollWidth: document.documentElement.scrollWidth,
      bodyScrollWidth: document.body.scrollWidth,
      viewportWidth,
      hasPageX: document.documentElement.scrollWidth > viewportWidth + 2 || document.body.scrollWidth > viewportWidth + 2,
      uncontainedOverflow: overflow.filter((entry) => !entry.contained),
      containedOverflow: overflow.filter((entry) => entry.contained).slice(0, 8),
      scrollbars,
    };
  }, scrollContainers);
}

function numericCssPx(value) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function scrollbarTooLarge(entry, maxPx) {
  const width = numericCssPx(entry.width);
  const height = numericCssPx(entry.height);
  return (width !== null && width > maxPx) || (height !== null && height > maxPx);
}

async function assertNativeScrollbars(page, label) {
  const selector = nativeScrollTargets;
  const targetIndexes = await page.evaluate((candidateSelector) => {
    return Array.from(document.querySelectorAll(candidateSelector)).map((element, index) => {
      const hasY = element.scrollHeight > element.clientHeight + 2;
      const hasX = element.scrollWidth > element.clientWidth + 2;
      return hasY || hasX ? index : -1;
    }).filter((index) => index >= 0);
  }, selector);
  if (targetIndexes.length === 0) return null;

  const reports = [];
  for (const targetIndex of targetIndexes) {
    const target = page.locator(selector).nth(targetIndex);
    const beforeHover = await target.evaluate((element) => {
      const styles = getComputedStyle(element, "::-webkit-scrollbar");
      return {
        width: styles.width,
        height: styles.height,
      };
    });
    await target.hover({ force: true });
    await page.waitForTimeout(40);
    reports.push(await target.evaluate((element, beforeHover) => {
      const styles = getComputedStyle(element, "::-webkit-scrollbar");
      const rect = element.getBoundingClientRect();
      const hasX = element.scrollWidth > element.clientWidth + 2;
      const hasY = element.scrollHeight > element.clientHeight + 2;
      const originalLeft = element.scrollLeft;
      const originalTop = element.scrollTop;
      if (hasX) element.scrollLeft = Math.min(12, element.scrollWidth - element.clientWidth);
      if (hasY) element.scrollTop = Math.min(12, element.scrollHeight - element.clientHeight);
      const movedX = !hasX || element.scrollLeft > originalLeft;
      const movedY = !hasY || element.scrollTop > originalTop;
      element.scrollLeft = originalLeft;
      element.scrollTop = originalTop;
      return {
        className: String(element.className || element.tagName),
        defaultWidth: beforeHover.width,
        defaultHeight: beforeHover.height,
        hoverWidth: styles.width,
        hoverHeight: styles.height,
        hasX,
        hasY,
        movedX,
        movedY,
        rect: { width: Math.round(rect.width), height: Math.round(rect.height) },
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
        scrollHeight: element.scrollHeight,
        clientHeight: element.clientHeight,
      };
    }, beforeHover));
  }
  const missingOrStatic = reports.filter((entry) => {
    const defaultWidth = numericCssPx(entry.defaultWidth);
    const defaultHeight = numericCssPx(entry.defaultHeight);
    const defaultXVisible = !entry.hasX || (defaultHeight !== null && defaultHeight > 0);
    const defaultYVisible = !entry.hasY || (defaultWidth !== null && defaultWidth > 0);
    return !defaultXVisible || !defaultYVisible || !entry.movedX || !entry.movedY;
  });
  const oversized = reports.filter((entry) => (
    scrollbarTooLarge({ width: entry.defaultWidth, height: entry.defaultHeight }, 4.5) ||
    scrollbarTooLarge({ width: entry.hoverWidth, height: entry.hoverHeight }, 5.5)
  ));
  assert(missingOrStatic.length === 0, `${label} has scrollable areas without visible working scrollbars`, missingOrStatic);
  assert(oversized.length === 0, `${label} has scrollbars thicker than the compact native style`, oversized);
  await page.mouse.move(1, 1);
  await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }));
  await page.waitForTimeout(40);
  return reports;
}

async function assertCriticalTextFits(page, label) {
  const { clips, connectionOverflow, connectionRowAlignment, vaultAlignment, stopAllTextStack, tightModeSegments } = await page.evaluate(() => {
    const selector = [
      ".source-section .segmented button",
      ".metric-number",
      ".metric-unit",
      ".stop-all span",
      ".stop-all small",
      ".connection-name",
      ".connection strong",
      ".connection-link strong",
    ].join(",");
    const clips = Array.from(document.querySelectorAll(selector)).map((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const text = (element.textContent || "").replace(/\s+/g, " ").trim();
      return {
        tag: element.tagName,
        className: String(element.className || ""),
        text,
        visible: styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1,
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      };
    }).filter((entry) => (
      entry.visible &&
      entry.text &&
      (entry.scrollWidth > entry.clientWidth + 1 || entry.scrollHeight > entry.clientHeight + 1)
    ));
    const connection = document.querySelector(".connection");
    const connectionRect = connection?.getBoundingClientRect();
    const connectionOverflow = !connection || !connectionRect ? [] : Array.from(connection.children).map((element) => {
      const rect = element.getBoundingClientRect();
      return {
        tag: element.tagName,
        className: String(element.className || ""),
        text: (element.textContent || "").replace(/\s+/g, " ").trim(),
        top: Math.round(rect.top),
        bottom: Math.round(rect.bottom),
        parentTop: Math.round(connectionRect.top),
        parentBottom: Math.round(connectionRect.bottom),
      };
    }).filter((entry) => entry.top < connectionRect.top - 1 || entry.bottom > connectionRect.bottom + 1);
    const connectionRows = connection
      ? Array.from(connection.querySelectorAll(":scope > div, :scope > button.connection-link"))
      : [];
    const rowMetrics = connectionRows.map((row) => {
      const icon = row.firstElementChild;
      const name = row.querySelector(".connection-name");
      const value = row.querySelector("strong");
      const rowRect = row.getBoundingClientRect();
      const iconRect = icon?.getBoundingClientRect();
      const nameRect = name?.getBoundingClientRect();
      const valueRect = value?.getBoundingClientRect();
      return {
        text: (row.textContent || "").replace(/\s+/g, " ").trim(),
        rowTop: Number(rowRect.top.toFixed(1)),
        rowBottom: Number(rowRect.bottom.toFixed(1)),
        iconCenterX: iconRect ? Number(((iconRect.left + iconRect.right) / 2).toFixed(1)) : null,
        iconTop: iconRect ? Number(iconRect.top.toFixed(1)) : null,
        iconBottom: iconRect ? Number(iconRect.bottom.toFixed(1)) : null,
        nameLeft: nameRect ? Number(nameRect.left.toFixed(1)) : null,
        valueRight: valueRect ? Number(valueRect.right.toFixed(1)) : null,
        iconEscapesRow: Boolean(iconRect && (iconRect.top < rowRect.top - 1 || iconRect.bottom > rowRect.bottom + 1)),
      };
    });
    const connectionRowAlignment = [];
    if (rowMetrics.length >= 3) {
      const completeRows = rowMetrics.filter((row) => row.iconCenterX !== null && row.nameLeft !== null && row.valueRight !== null);
      if (completeRows.length !== rowMetrics.length) {
        connectionRowAlignment.push({ reason: "missing row metric", rows: rowMetrics });
      } else {
        const base = completeRows[0];
        const drift = completeRows.map((row) => ({
          text: row.text,
          iconDrift: Number(Math.abs(row.iconCenterX - base.iconCenterX).toFixed(1)),
          nameDrift: Number(Math.abs(row.nameLeft - base.nameLeft).toFixed(1)),
          valueDrift: Number(Math.abs(row.valueRight - base.valueRight).toFixed(1)),
          iconEscapesRow: row.iconEscapesRow,
        })).filter((row) => row.iconDrift > 1.5 || row.nameDrift > 1.5 || row.valueDrift > 1.5 || row.iconEscapesRow);
        connectionRowAlignment.push(...drift);
      }
    }
    const vaultLink = document.querySelector(".connection-vault-link");
    const vaultIcon = vaultLink?.querySelector("svg");
    const vaultName = vaultLink?.querySelector(".connection-name");
    const vaultCount = vaultLink?.querySelector("strong");
    let vaultAlignment = [];
    if (vaultLink && vaultIcon && vaultName && vaultCount) {
      const linkRect = vaultLink.getBoundingClientRect();
      const iconRect = vaultIcon.getBoundingClientRect();
      const nameRect = vaultName.getBoundingClientRect();
      const countRect = vaultCount.getBoundingClientRect();
      const textRange = document.createRange();
      textRange.selectNodeContents(vaultName);
      const textRect = textRange.getBoundingClientRect();
      textRange.detach();
      const iconNameGap = nameRect.left - iconRect.right;
      const iconTextGap = textRect.left - iconRect.right;
      const nameLeftOffset = nameRect.left - linkRect.left;
      const countRightGap = linkRect.right - countRect.right;
      const nameCountGap = countRect.left - nameRect.right;
      if (iconNameGap > 14 || iconTextGap > 14 || nameLeftOffset > 34 || countRightGap < -1 || nameCountGap < 4) {
        vaultAlignment = [{
          iconNameGap: Math.round(iconNameGap),
          iconTextGap: Math.round(iconTextGap),
          nameLeftOffset: Math.round(nameLeftOffset),
          countRightGap: Math.round(countRightGap),
          nameCountGap: Math.round(nameCountGap),
          linkWidth: Math.round(linkRect.width),
          text: (vaultLink.textContent || "").replace(/\s+/g, " ").trim(),
        }];
      }
    }
    const stopAll = document.querySelector(".stop-all");
    const stopTitle = stopAll?.querySelector("span");
    const stopSub = stopAll?.querySelector("small");
    let stopAllTextStack = [];
    if (stopAll && stopTitle && stopSub) {
      const titleRect = stopTitle.getBoundingClientRect();
      const subRect = stopSub.getBoundingClientRect();
      const gap = subRect.top - titleRect.bottom;
      if (gap < 2) {
        stopAllTextStack = [{
          title: stopTitle.textContent?.replace(/\s+/g, " ").trim() || "",
          sub: stopSub.textContent?.replace(/\s+/g, " ").trim() || "",
          gap: Number(gap.toFixed(1)),
          titleBottom: Math.round(titleRect.bottom),
          subTop: Math.round(subRect.top),
        }];
      }
    }
    const tightModeSegments = Array.from(document.querySelectorAll(".mode-section .segmented button")).map((element) => {
      const range = document.createRange();
      range.selectNodeContents(element);
      const textRect = range.getBoundingClientRect();
      const rect = element.getBoundingClientRect();
      range.detach();
      return {
        text: (element.textContent || "").replace(/\s+/g, " ").trim(),
        width: Math.round(rect.width),
        textWidth: Math.round(textRect.width),
        sideBreathing: Math.round((rect.width - textRect.width) / 2),
      };
    }).filter((entry) => entry.text && entry.sideBreathing < 3);
    return { clips, connectionOverflow, connectionRowAlignment, vaultAlignment, stopAllTextStack, tightModeSegments };
  });
  assert(clips.length === 0, `${label} has clipped critical top-bar text`, clips);
  assert(connectionOverflow.length === 0, `${label} has vertically clipped connection rows`, connectionOverflow);
  assert(connectionRowAlignment.length === 0, `${label} has misaligned connection status rows`, connectionRowAlignment);
  assert(vaultAlignment.length === 0, `${label} has misaligned vault connection row`, vaultAlignment);
  assert(stopAllTextStack.length === 0, `${label} stop-all button text is too vertically tight`, stopAllTextStack);
  assert(tightModeSegments.length === 0, `${label} has cramped mode segmented labels`, tightModeSegments);
}

async function assertStateTextContrast(page, label) {
  const report = await page.evaluate(() => {
    const parseColor = (input) => {
      const match = String(input || "").match(/rgba?\(([^)]+)\)/);
      if (!match) return null;
      const parts = match[1].split(",").map((value) => Number.parseFloat(value.trim()));
      return {
        r: parts[0],
        g: parts[1],
        b: parts[2],
        a: Number.isFinite(parts[3]) ? parts[3] : 1,
      };
    };
    const blend = (front, back) => {
      const alpha = front.a;
      return {
        r: front.r * alpha + back.r * (1 - alpha),
        g: front.g * alpha + back.g * (1 - alpha),
        b: front.b * alpha + back.b * (1 - alpha),
        a: 1,
      };
    };
    const relativeLuminance = (color) => {
      const channels = [color.r, color.g, color.b].map((value) => {
        const normalized = value / 255;
        return normalized <= 0.03928 ? normalized / 12.92 : Math.pow((normalized + 0.055) / 1.055, 2.4);
      });
      return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
    };
    const contrast = (first, second) => {
      const a = relativeLuminance(first);
      const b = relativeLuminance(second);
      return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
    };
    const effectiveBackground = (element) => {
      const rootBackground = getComputedStyle(document.documentElement).getPropertyValue("--surface").trim();
      let background = parseColor(rootBackground) || { r: 255, g: 255, b: 255, a: 1 };
      const stack = [];
      for (let node = element; node && node.nodeType === 1; node = node.parentElement) {
        stack.push(node);
      }
      for (let index = stack.length - 1; index >= 0; index -= 1) {
        const color = parseColor(getComputedStyle(stack[index]).backgroundColor);
        if (color && color.a > 0) background = blend(color, background);
      }
      return background;
    };
    const selectors = [
      ".metric-value",
      ".metric-unit",
      ".metric-tile small",
      ".positive-text",
      ".negative-text",
      ".warn-text",
      ".connection strong",
      ".connection-link strong",
      ".status-pill",
    ].join(",");
    const failures = [];
    const seen = new Set();
    for (const element of Array.from(document.querySelectorAll(selectors))) {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const text = (element.textContent || "").replace(/\s+/g, " ").trim();
      const key = `${Math.round(rect.left)}:${Math.round(rect.top)}:${text}`;
      if (!text || seen.has(key) || rect.width < 2 || rect.height < 2 || rect.bottom < 0 || rect.top > window.innerHeight || styles.display === "none" || styles.visibility === "hidden") continue;
      seen.add(key);
      const foreground = parseColor(styles.color);
      if (!foreground) continue;
      const background = effectiveBackground(element);
      const ratio = contrast(foreground.a < 1 ? blend(foreground, background) : foreground, background);
      const fontSize = Number.parseFloat(styles.fontSize) || 12;
      const fontWeight = Number.parseInt(styles.fontWeight, 10) || 400;
      const minimum = fontSize >= 18 || (fontSize >= 14 && fontWeight >= 600) ? 3 : 4.5;
      if (ratio < minimum) {
        failures.push({
          text: text.slice(0, 80),
          className: String(element.className || element.tagName).slice(0, 100),
          ratio: Number(ratio.toFixed(2)),
          minimum,
          color: styles.color,
          background: getComputedStyle(element).backgroundColor,
          top: Math.round(rect.top),
          left: Math.round(rect.left),
          fontSize,
          fontWeight,
        });
      }
    }
    return failures;
  });
  assert(report.length === 0, `${label} has low-contrast state text`, report.slice(0, 12));
  return { checked: true };
}

async function assertPrimaryHitTargets(page, label) {
  const report = await page.evaluate(() => {
    const selectors = [
      ".guard-link",
      ".connection-link",
      ".filter-row button",
    ];
    return selectors.flatMap((selector) => Array.from(document.querySelectorAll(selector)).map((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      return {
        selector,
        text: (element.textContent || element.getAttribute("aria-label") || element.getAttribute("title") || "").replace(/\s+/g, " ").trim().slice(0, 80),
        width: Number(rect.width.toFixed(1)),
        height: Number(rect.height.toFixed(1)),
        visible: rect.width > 1 && rect.height > 1 && rect.bottom > 0 && rect.top < window.innerHeight && styles.display !== "none" && styles.visibility !== "hidden",
      };
    })).filter((entry) => {
      if (!entry.visible) return false;
      const minimum = entry.selector === ".connection-link" ? 20 : 24;
      return entry.height < minimum;
    });
  });
  assert(report.length === 0, `${label} has high-frequency controls with undersized touch targets`, report);
  return { checked: true };
}

async function assertScrollableTabAffordance(page, label, viewportName) {
  if (viewportName !== "mobile") return null;
  const report = await page.evaluate(() => {
    return Array.from(document.querySelectorAll(".workspace-tabs,.bottom-tabs")).map((element) => {
      const styles = getComputedStyle(element);
      return {
        className: String(element.className || ""),
        scrollWidth: element.scrollWidth,
        clientWidth: element.clientWidth,
        overflowX: styles.overflowX,
        maskImage: styles.maskImage || styles.webkitMaskImage || "",
        webkitMaskImage: styles.webkitMaskImage || "",
      };
    }).filter((entry) => entry.scrollWidth > entry.clientWidth + 2);
  });
  const missing = report.filter((entry) => {
    const mask = `${entry.maskImage} ${entry.webkitMaskImage}`;
    return !mask || /\\bnone\\b/i.test(mask);
  });
  assert(missing.length === 0, `${label} scrollable tab bars lack a horizontal affordance`, missing);
  return report;
}

async function assertScrollableTabActivation(page, label, viewportName) {
  if (viewportName !== "mobile") return null;
  const groups = [
    { name: "workspace", container: ".workspace-tabs", buttons: ".workspace-tabs button" },
    { name: "bottom", container: ".bottom-tabs", buttons: ".bottom-tabs button" },
  ];
  const results = [];
  for (const group of groups) {
    const count = await page.locator(group.buttons).count();
    assert(count > 0, `${label} ${group.name} tab group is missing`, group);
    for (let index = 0; index < count; index += 1) {
      await page.locator(group.buttons).nth(index).click();
      await page.waitForTimeout(100);
      const report = await page.evaluate(({ group, index }) => {
        const container = document.querySelector(group.container);
        const buttons = Array.from(document.querySelectorAll(group.buttons));
        const button = buttons[index];
        if (!container || !button) return { missing: true, index };
        const containerRect = container.getBoundingClientRect();
        const buttonRect = button.getBoundingClientRect();
        return {
          missing: false,
          index,
          text: (button.textContent || "").replace(/\s+/g, " ").trim(),
          activeIndex: buttons.findIndex((element) => element.classList.contains("active")),
          scrollLeft: Math.round(container.scrollLeft),
          leftInset: Math.round(buttonRect.left - containerRect.left),
          rightInset: Math.round(containerRect.right - buttonRect.right),
          fullyVisible: buttonRect.left >= containerRect.left - 1 && buttonRect.right <= containerRect.right + 1,
        };
      }, { group, index });
      assert(!report.missing, `${label} ${group.name} tab ${index} is missing after click`, report);
      assert(report.activeIndex === index, `${label} ${group.name} tab did not activate`, report);
      assert(report.fullyVisible, `${label} ${group.name} active tab is not fully visible after activation`, report);
      results.push({ group: group.name, ...report });
    }
  }
  await page.locator(".workspace-tabs button").first().click();
  await page.waitForTimeout(80);
  await page.locator(".bottom-tabs button").first().click();
  await page.waitForTimeout(80);
  await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }));
  await page.waitForTimeout(80);
  return results;
}

async function assertBrandScale(page, label) {
  const report = await page.evaluate(() => {
    const block = document.querySelector(".brand-block");
    const logo = document.querySelector(".brand-icon img");
    const title = document.querySelector(".brand-block h1");
    const themeTrigger = document.querySelector(".theme-trigger");
    const languageTrigger = document.querySelector(".language-trigger");
    if (!block || !logo || !title || !themeTrigger || !languageTrigger) return { missing: true };
    const blockRect = block.getBoundingClientRect();
    const logoRect = logo.getBoundingClientRect();
    const titleRect = title.getBoundingClientRect();
    const themeRect = themeTrigger.getBoundingClientRect();
    const languageRect = languageTrigger.getBoundingClientRect();
    const titleStyle = getComputedStyle(title);
    return {
      missing: false,
      blockHeight: Math.round(blockRect.height),
      logoWidth: Math.round(logoRect.width),
      logoHeight: Math.round(logoRect.height),
      titleWidth: Math.round(titleRect.width),
      titleHeight: Math.round(titleRect.height),
      titleTop: Math.round(titleRect.top - blockRect.top),
      titleBottom: Math.round(titleRect.bottom - blockRect.top),
      titleVisible: titleRect.width > 44 && titleRect.height > 9 && titleStyle.display !== "none" && titleStyle.visibility !== "hidden",
      titleFontSize: Number.parseFloat(titleStyle.fontSize),
      themeTriggerHeight: Math.round(themeRect.height),
      languageTriggerHeight: Math.round(languageRect.height),
    };
  });
  assert(!report.missing, `${label} is missing brand scale elements`, report);
  assert(report.logoWidth <= 36 && report.logoHeight <= 36, `${label} brand logo is too visually dominant`, report);
  assert(report.blockHeight <= 52, `${label} brand block is taller than the compact target`, report);
  assert(report.titleVisible, `${label} brand title is not visibly preserved`, report);
  assert(report.titleTop >= 0 && report.titleBottom <= report.blockHeight, `${label} brand title is clipped by the brand block`, report);
  assert(report.titleFontSize <= 15, `${label} brand title is too visually dominant`, report);
  assert(report.themeTriggerHeight <= 18 && report.languageTriggerHeight <= 18, `${label} brand theme/language switchers are too visually dominant`, report);
  return report;
}

async function assertRunningMotion(page, label) {
  const report = await page.evaluate(() => {
    const visible = (element) => {
      if (!element) return false;
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      return styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1;
    };
    const runningPills = Array.from(document.querySelectorAll(".status-pill.running")).filter(visible).map((element) => ({
      text: (element.textContent || "").replace(/\s+/g, " ").trim(),
      animationName: getComputedStyle(element).animationName,
      animationDuration: getComputedStyle(element).animationDuration,
    }));
    const runningRows = Array.from(document.querySelectorAll(".run-row.running")).filter(visible).map((element) => ({
      element,
      rect: element.getBoundingClientRect(),
    })).map(({ element, rect }) => {
      const sweep = getComputedStyle(element, "::after");
      const top = Number.parseFloat(sweep.top) || 0;
      const bottom = Number.parseFloat(sweep.bottom) || 0;
      const left = Number.parseFloat(sweep.left) || 0;
      const right = Number.parseFloat(sweep.right) || 0;
      const sweepHeight = Math.max(0, rect.height - top - bottom);
      const sweepWidth = Math.max(0, rect.width - left - right);
      return {
        text: (element.textContent || "").replace(/\s+/g, " ").trim().slice(0, 80),
        rowAnimationName: getComputedStyle(element).animationName,
        sweepAnimationName: sweep.animationName,
        sweepOpacity: sweep.opacity,
        sweepCoverageY: Number((sweepHeight / rect.height).toFixed(2)),
        sweepCoverageX: Number((sweepWidth / rect.width).toFixed(2)),
      };
    });
    return {
      runningPills,
      runningRows,
      missingPillAnimation: runningPills.filter((entry) => entry.animationName === "none"),
      missingRowSweep: runningRows.filter((entry) => entry.sweepAnimationName === "none"),
      oversizedRowSweep: runningRows.filter((entry) => entry.sweepCoverageY > 0.82 || entry.sweepCoverageX > 0.98),
    };
  });
  assert(report.runningPills.length > 0, `${label} has no visible running status pill to animate`, report);
  assert(report.runningRows.length > 0, `${label} has no visible running row affordance`, report);
  assert(report.missingPillAnimation.length === 0, `${label} running status pills are static`, report.missingPillAnimation);
  assert(report.missingRowSweep.length === 0, `${label} running rows are static`, report.missingRowSweep);
  assert(report.oversizedRowSweep.length === 0, `${label} running row sweep is too visually heavy`, report.oversizedRowSweep);
  return report;
}

async function assertLightSegmentedChrome(page, label) {
  if (!label.includes("-light-")) return null;
  const report = await page.evaluate(() => {
    const alphaOf = (input) => {
      const match = String(input || "").match(/rgba?\(([^)]+)\)/i);
      if (!match) return 1;
      const parts = match[1].split(",").map((part) => part.trim());
      return parts.length >= 4 ? Number.parseFloat(parts[3]) : 1;
    };
    const outer = Array.from(document.querySelectorAll(".segmented")).map((element) => {
      const styles = getComputedStyle(element);
      const active = element.querySelector("button.active");
      const divider = element.querySelector("button + button");
      return {
        className: String(element.className || ""),
        borderColor: styles.borderColor,
        borderAlpha: alphaOf(styles.borderColor),
        activeBackground: active ? getComputedStyle(active).backgroundColor : "",
        activeBackgroundAlpha: active ? alphaOf(getComputedStyle(active).backgroundColor) : 0,
        dividerColor: divider ? getComputedStyle(divider).borderLeftColor : "",
        dividerAlpha: divider ? alphaOf(getComputedStyle(divider).borderLeftColor) : 0,
      };
    });
    return outer;
  });
  const noisy = report.filter((entry) => (
    entry.borderAlpha > 0.12 ||
    entry.dividerAlpha > 0.1 ||
    entry.activeBackgroundAlpha > 0.085
  ));
  assert(noisy.length === 0, `${label} light segmented controls are visually too heavy`, noisy);
  return report;
}

async function assertTopBarLayer(page, label) {
  await page.evaluate(() => window.scrollTo({ top: 0, left: 0, behavior: "instant" }));
  await page.waitForTimeout(40);
  const report = await page.evaluate(() => {
    const alphaValues = (input) => {
      const values = [];
      const pattern = /rgba?\(([^)]+)\)/gi;
      let match;
      while ((match = pattern.exec(String(input || ""))) !== null) {
        const parts = match[1].split(",").map((part) => part.trim());
        values.push(parts.length >= 4 ? Number.parseFloat(parts[3]) : 1);
      }
      return values;
    };
    const topBar = document.querySelector(".top-bar");
    const labGrid = document.querySelector(".lab-grid");
    const shell = document.querySelector(".app-shell");
    if (!topBar || !labGrid || !shell) return { missing: true };
    const topRect = topBar.getBoundingClientRect();
    const labRect = labGrid.getBoundingClientRect();
    const topStyles = getComputedStyle(topBar);
    const shellStyles = getComputedStyle(shell);
    const topAlphas = [
      ...alphaValues(topStyles.backgroundColor),
      ...alphaValues(topStyles.backgroundImage),
    ];
    const shellAlphas = [
      ...alphaValues(shellStyles.backgroundColor),
      ...alphaValues(shellStyles.backgroundImage),
    ];
    const topLayerAlphas = alphaValues(topStyles.backgroundImage);
    const topColorAlphas = alphaValues(topStyles.backgroundColor);
    const shellLayerAlphas = alphaValues(shellStyles.backgroundImage);
    const shellColorAlphas = alphaValues(shellStyles.backgroundColor);
    const topOpaqueLayer = topLayerAlphas.some((alpha) => alpha >= 0.995) || topColorAlphas.some((alpha) => alpha >= 0.995);
    const shellOpaqueLayer = shellLayerAlphas.some((alpha) => alpha >= 0.995) || shellColorAlphas.some((alpha) => alpha >= 0.995);
    return {
      missing: false,
      topBar: {
        top: Math.round(topRect.top),
        bottom: Math.round(topRect.bottom),
        height: Math.round(topRect.height),
        position: topStyles.position,
        backgroundColor: topStyles.backgroundColor,
        backgroundImage: topStyles.backgroundImage,
        minAlpha: topAlphas.length ? Math.min(...topAlphas) : 1,
        opaqueLayer: topOpaqueLayer,
      },
      shell: {
        backgroundColor: shellStyles.backgroundColor,
        backgroundImage: shellStyles.backgroundImage,
        minAlpha: shellAlphas.length ? Math.min(...shellAlphas) : 1,
        opaqueLayer: shellOpaqueLayer,
      },
      labGrid: {
        top: Math.round(labRect.top),
      },
      labGridGap: Math.round(labRect.top - topRect.bottom),
    };
  });
  assert(!report.missing, `${label} is missing top bar layer elements`, report);
  assert(report.labGridGap >= -1, `${label} content starts underneath the top bar`, report);
  if (label.includes("-tablet-") || label.includes("-breakpoint-")) {
    assert(report.topBar.height <= 160, `${label} tablet top bar is too tall and wastes first-viewport space`, report.topBar);
  }
  if (label.includes("-mobile-")) {
    assert(report.topBar.position !== "sticky", `${label} mobile top bar should scroll away instead of occupying the viewport`, report.topBar);
  }
  if (label.includes("-light-")) {
    assert(report.topBar.opaqueLayer, `${label} light top bar lets chart lines bleed through`, report.topBar);
    assert(report.shell.opaqueLayer, `${label} light app shell is translucent`, report.shell);
  }
  return report;
}

async function assertLeftRailStack(page, label, viewportName, options = {}) {
  if (viewportName !== "desktop") return null;
  const minFullRows = options.minFullRows ?? 5;

  const report = await page.evaluate(() => {
    const rectFor = (selector) => {
      const element = document.querySelector(selector);
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      return {
        top: Number(rect.top.toFixed(1)),
        bottom: Number(rect.bottom.toFixed(1)),
        height: Number(rect.height.toFixed(1)),
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      };
    };
    const controls = document.querySelector(".controls-panel");
    const controlsRect = controls?.getBoundingClientRect();
    const controlsContentBottom = controls && controlsRect ? Math.max(
      ...Array.from(controls.querySelectorAll("*")).map((element) => {
        const styles = getComputedStyle(element);
        const rect = element.getBoundingClientRect();
        const hidden = styles.display === "none" || styles.visibility === "hidden" || rect.width <= 1 || rect.height <= 1;
        return hidden ? controlsRect.top : rect.bottom;
      })
    ) : 0;
    const jump = document.querySelector(".jump-control")?.getBoundingClientRect();
    const latency = document.querySelector(".latency-strip")?.getBoundingClientRect();
    const runsPanel = document.querySelector(".runs-panel");
    const archiveDrawer = document.querySelector(".archive-drawer");
    const archiveRow = document.querySelector(".archive-row");
    const archiveCount = document.querySelector(".archive-count");
    const runList = document.querySelector(".run-list");
    const runListStyles = runList ? getComputedStyle(runList) : null;
    const runsPanelRect = runsPanel?.getBoundingClientRect();
    const runListRect = runList?.getBoundingClientRect();
    const archiveDrawerRect = archiveDrawer?.getBoundingClientRect();
    const archiveRowRect = archiveRow?.getBoundingClientRect();
    const archiveCountRect = archiveCount?.getBoundingClientRect();
    const archiveRowStyles = archiveRow ? getComputedStyle(archiveRow) : null;
    const archiveStackBottom = archiveDrawerRect?.bottom || archiveRowRect?.bottom || null;
    const runRows = runList && runListRect ? Array.from(runList.querySelectorAll(".run-row")).map((element) => {
      const rect = element.getBoundingClientRect();
      const visible = Math.max(0, Math.min(rect.bottom, runListRect.bottom) - Math.max(rect.top, runListRect.top));
      return {
        top: Number(rect.top.toFixed(1)),
        bottom: Number(rect.bottom.toFixed(1)),
        height: Number(rect.height.toFixed(1)),
        visible: Number(visible.toFixed(1)),
        escapedRunList: visible > 1 && (
          rect.top < runListRect.top - 1 ||
          rect.bottom > runListRect.bottom + 1
        ),
      };
    }) : [];
    const firstRowHeight = runRows.find((row) => row.height > 1)?.height || 1;
    const visibleRowEquivalent = runRows.reduce((sum, row) => sum + row.visible, 0) / firstRowHeight;
    const partialRows = runRows.filter((row) => row.visible > 1 && row.visible < row.height - 1);
    const directChildren = Array.from(document.querySelectorAll(".left-rail > *")).map((element) => {
      const rect = element.getBoundingClientRect();
      return {
        className: String(element.className || element.tagName),
        top: Number(rect.top.toFixed(1)),
        bottom: Number(rect.bottom.toFixed(1)),
        height: Number(rect.height.toFixed(1)),
      };
    });
    const childOverlaps = directChildren.slice(1).map((entry, index) => ({
      previous: directChildren[index],
      current: entry,
      gap: Number((entry.top - directChildren[index].bottom).toFixed(1)),
    })).filter((entry) => entry.gap < 4);
    const internalStack = {
      runsPanel: runsPanelRect ? {
        top: Number(runsPanelRect.top.toFixed(1)),
        bottom: Number(runsPanelRect.bottom.toFixed(1)),
        height: Number(runsPanelRect.height.toFixed(1)),
      } : null,
      runList: runListRect ? {
        top: Number(runListRect.top.toFixed(1)),
        bottom: Number(runListRect.bottom.toFixed(1)),
        height: Number(runListRect.height.toFixed(1)),
      } : null,
      archiveDrawer: archiveDrawerRect ? {
        top: Number(archiveDrawerRect.top.toFixed(1)),
        bottom: Number(archiveDrawerRect.bottom.toFixed(1)),
        height: Number(archiveDrawerRect.height.toFixed(1)),
        text: (archiveDrawer.textContent || "").replace(/\s+/g, " ").trim(),
      } : null,
      archiveRow: archiveRowRect ? {
        top: Number(archiveRowRect.top.toFixed(1)),
        bottom: Number(archiveRowRect.bottom.toFixed(1)),
        height: Number(archiveRowRect.height.toFixed(1)),
        fontSize: Number.parseFloat(archiveRowStyles?.fontSize || "0"),
        text: (archiveRow.textContent || "").replace(/\s+/g, " ").trim(),
      } : null,
      archiveCount: archiveCountRect ? {
        top: Number(archiveCountRect.top.toFixed(1)),
        bottom: Number(archiveCountRect.bottom.toFixed(1)),
        height: Number(archiveCountRect.height.toFixed(1)),
        width: Number(archiveCountRect.width.toFixed(1)),
        text: (archiveCount.textContent || "").replace(/\s+/g, " ").trim(),
      } : null,
      runListToArchiveGap: runListRect && archiveRowRect
        ? Number((archiveRowRect.top - runListRect.bottom).toFixed(1))
        : null,
      archiveToDrawerGap: archiveDrawerRect && archiveRowRect
        ? Number((archiveDrawerRect.top - archiveRowRect.bottom).toFixed(1))
        : null,
      archiveDrawerBelowRow: Boolean(!archiveDrawerRect || !archiveRowRect || archiveDrawerRect.top >= archiveRowRect.bottom - 1),
      archiveToControlsGap: archiveStackBottom && controlsRect
        ? Number((controlsRect.top - archiveStackBottom).toFixed(1))
        : null,
      archiveWithinPanel: Boolean(runsPanelRect && archiveRowRect &&
        archiveRowRect.top >= runsPanelRect.top - 1 &&
        (archiveStackBottom || archiveRowRect.bottom) <= runsPanelRect.bottom + 1),
      controlsBelowArchive: Boolean(archiveStackBottom && controlsRect && controlsRect.top >= archiveStackBottom + 4),
      visibleRowsEscapingRunList: runRows.filter((row) => row.escapedRunList).map((row) => ({
        top: row.top,
        bottom: row.bottom,
        height: row.height,
        visible: row.visible,
      })),
    };
    return {
      left: rectFor(".left-rail"),
      workspace: rectFor(".workspace"),
      controls: rectFor(".controls-panel"),
      jump: rectFor(".jump-control"),
      latency: rectFor(".latency-strip"),
      controlsContentOverflow: controlsRect ? Number((controlsContentBottom - controlsRect.bottom).toFixed(1)) : null,
      jumpToLatencyGap: jump && latency ? Number((latency.top - jump.bottom).toFixed(1)) : null,
      latencyViewportGap: latency ? Number((window.innerHeight - latency.bottom).toFixed(1)) : null,
      latencyRailBottomGap: latency ? Number((rectFor(".left-rail").bottom - latency.bottom).toFixed(1)) : null,
      latencyWorkspaceBottomGap: latency ? Number((rectFor(".workspace").bottom - latency.bottom).toFixed(1)) : null,
      runListDensity: {
        listHeight: runListRect ? Number(runListRect.height.toFixed(1)) : 0,
        overflowX: runListStyles?.overflowX || "",
        scrollWidth: runList?.scrollWidth || 0,
        clientWidth: runList?.clientWidth || 0,
        rowHeight: firstRowHeight,
        visibleRowEquivalent: Number(visibleRowEquivalent.toFixed(2)),
        fullRows: runRows.filter((row) => row.visible >= row.height - 1).length,
        partialRows: partialRows.map((row) => ({
          height: row.height,
          visible: row.visible,
        })),
      },
      internalStack,
      childOverlaps,
    };
  });
  assert(report.controls && report.jump && report.latency, `${label} is missing left rail stack elements`, report);
  assert(report.controlsContentOverflow <= 1, `${label} controls panel content overflows its container`, report);
  assert(report.jumpToLatencyGap >= 4, `${label} jump control overlaps the latency strip`, report);
  assert(report.latencyViewportGap >= 4, `${label} latency strip is too close to the viewport edge`, report);
  assert(Math.abs(report.latencyRailBottomGap) <= 2, `${label} latency strip does not align with the left rail bottom`, report);
  assert(Math.abs(report.latencyWorkspaceBottomGap) <= 2, `${label} left rail bottom no longer aligns with the workspace bottom`, report);
  assert(report.runListDensity.fullRows >= minFullRows, `${label} experiment run list shows too few complete rows`, report.runListDensity);
  assert(report.runListDensity.partialRows.length === 0, `${label} experiment run list shows a clipped partial row`, report.runListDensity);
  assert(report.runListDensity.overflowX === "hidden", `${label} experiment run list should not show a horizontal scrollbar`, report.runListDensity);
  assert(report.internalStack?.archiveWithinPanel, `${label} archived row escapes the experiment runs panel`, report.internalStack);
  if (report.internalStack?.runListToArchiveGap !== null) {
    assert(report.internalStack.runListToArchiveGap >= -1, `${label} archived row overlaps the visible run list`, report.internalStack);
  }
  if (report.internalStack?.archiveToDrawerGap !== null) {
    assert(report.internalStack.archiveToDrawerGap >= -1, `${label} archived drawer overlaps the archived trigger`, report.internalStack);
  }
  assert(report.internalStack?.archiveDrawerBelowRow, `${label} archived drawer should unfold below the archived trigger`, report.internalStack);
  assert(report.internalStack?.archiveRow?.height <= 32, `${label} archived trigger is too tall`, report.internalStack?.archiveRow);
  assert(report.internalStack?.archiveRow?.fontSize <= 12, `${label} archived trigger text is too large`, report.internalStack?.archiveRow);
  assert(report.internalStack?.archiveCount?.text === "12", `${label} archived count should be a separate compact badge`, report.internalStack?.archiveCount);
  assert(report.internalStack?.archiveCount?.height <= 18, `${label} archived count badge is too heavy`, report.internalStack?.archiveCount);
  if (report.internalStack?.archiveDrawer) {
    assert(!/\b12\b/.test(report.internalStack.archiveDrawer.text), `${label} archived drawer repeats the count instead of leaving it in the badge`, report.internalStack.archiveDrawer);
  }
  assert(report.internalStack?.controlsBelowArchive, `${label} controls panel is too close to the archived row`, report.internalStack);
  assert(report.internalStack?.archiveToControlsGap <= 16, `${label} archived row leaves too much blank space before controls`, report.internalStack);
  assert(report.internalStack?.visibleRowsEscapingRunList?.length === 0, `${label} visible run rows escape the clipped list container`, report.internalStack?.visibleRowsEscapingRunList);
  assert(report.childOverlaps.length === 0, `${label} left rail sections are visually cramped or overlapping`, report.childOverlaps);
  return report;
}

async function assertRunsPanelWhitespace(page, label) {
  const report = await page.evaluate(() => {
    const panel = document.querySelector(".runs-panel");
    const archiveRow = document.querySelector(".archive-row");
    const archiveDrawer = document.querySelector(".archive-drawer");
    const last = archiveDrawer || archiveRow;
    if (!panel || !last) return { missing: true };
    const panelRect = panel.getBoundingClientRect();
    const lastRect = last.getBoundingClientRect();
    const visibleRows = Array.from(document.querySelectorAll(".run-list .run-row")).map((element) => {
      const rect = element.getBoundingClientRect();
      const list = element.closest(".run-list")?.getBoundingClientRect();
      const visible = list ? Math.max(0, Math.min(rect.bottom, list.bottom) - Math.max(rect.top, list.top)) : rect.height;
      return {
        height: Number(rect.height.toFixed(1)),
        visible: Number(visible.toFixed(1)),
      };
    }).filter((row) => row.visible > 1);
    return {
      missing: false,
      panelHeight: Number(panelRect.height.toFixed(1)),
      lastClassName: String(last.className || ""),
      bottomGap: Number((panelRect.bottom - lastRect.bottom).toFixed(1)),
      visibleRows,
      partialRows: visibleRows.filter((row) => row.visible > 1 && row.visible < row.height - 1),
    };
  });
  assert(!report.missing, `${label} is missing the archived run disclosure`, report);
  assert(report.bottomGap <= 12, `${label} experiment runs panel leaves too much blank space after archive disclosure`, report);
  assert(report.partialRows.length === 0, `${label} experiment runs panel shows clipped partial rows`, report);
  return report;
}

async function assertArchiveDisclosure(page, label, viewportName) {
  const trigger = page.locator(".archive-row").first();
  assert(await trigger.isVisible().catch(() => false), `${label} archived trigger is missing`);
  const collapsedStack = await assertRunsPanelWhitespace(page, `${label} archive collapsed`);

  await trigger.click();
  await page.waitForTimeout(160);
  assert((await trigger.getAttribute("aria-expanded")) === "true", `${label} archived trigger did not expand`);
  assert(await page.locator(".archive-drawer").first().isVisible().catch(() => false), `${label} archived drawer did not appear`);
  const expandedDisclosureStack = await assertRunsPanelWhitespace(page, `${label} archive expanded`);
  const expandedStack = await assertLeftRailStack(page, `${label} archive expanded`, viewportName, { minFullRows: 4 });
  if (expandedStack) {
    assert(expandedStack.internalStack?.archiveToDrawerGap >= -1, `${label} archive drawer is not attached to trigger`, expandedStack.internalStack);
  }

  await trigger.click();
  await page.waitForTimeout(160);
  assert((await trigger.getAttribute("aria-expanded")) === "false", `${label} archived trigger did not collapse`);
  assert(!(await page.locator(".archive-drawer").first().isVisible().catch(() => false)), `${label} archived drawer remained visible after collapse`);
  await closeToasts(page);
  return { collapsedStack, expandedDisclosureStack, leftRailStack: expandedStack?.internalStack || null };
}

async function assertChartFooter(page, label) {
  const report = await page.evaluate(() => {
    const footer = document.querySelector(".market-footer");
    const chart = document.querySelector(".chart-workspace");
    if (!footer || !chart) return { missing: true };
    const footerRect = footer.getBoundingClientRect();
    const chartRect = chart.getBoundingClientRect();
    const items = Array.from(footer.children).map((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const text = (element.textContent || "").replace(/\s+/g, " ").trim();
      return {
        tag: element.tagName,
        className: String(element.className || ""),
        text,
        visible: styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1,
        left: Math.round(rect.left),
        right: Math.round(rect.right),
        top: Math.round(rect.top),
        bottom: Math.round(rect.bottom),
        clientWidth: element.clientWidth,
        scrollWidth: element.scrollWidth,
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
        clipped: element.scrollWidth > element.clientWidth + 1 || element.scrollHeight > element.clientHeight + 1,
        malformedSign: /(^|[^\d])[+-]{2}\d/.test(text),
      };
    }).filter((entry) => entry.visible);
    return {
      missing: false,
      footer: {
        left: Math.round(footerRect.left),
        right: Math.round(footerRect.right),
        bottom: Math.round(footerRect.bottom),
        chartBottomGap: Math.round(chartRect.bottom - footerRect.bottom),
      },
      items,
      clipped: items.filter((entry) => entry.clipped),
      malformedSigns: items.filter((entry) => entry.malformedSign),
      outsideFooter: items.filter((entry) => (
        entry.left < footerRect.left - 1 ||
        entry.right > footerRect.right + 1 ||
        entry.top < footerRect.top - 1 ||
        entry.bottom > footerRect.bottom + 1
      )),
    };
  });
  assert(!report.missing, `${label} is missing the market footer`, report);
  assert(report.footer.chartBottomGap >= 0, `${label} market footer escapes the chart panel`, report);
  assert(report.clipped.length === 0, `${label} market footer has clipped text`, report.clipped);
  assert(report.malformedSigns.length === 0, `${label} market footer has malformed signed values`, report.malformedSigns);
  assert(report.outsideFooter.length === 0, `${label} market footer children overflow their container`, report.outsideFooter);
  return report;
}

async function assertWorkspaceVerticalStack(page, label, viewportName) {
  if (viewportName === "tablet-portrait" || viewportName === "breakpoint-901" || viewportName === "breakpoint-761") {
    const report = await page.evaluate(() => {
      const rectFor = (selector) => {
        const element = document.querySelector(selector);
        if (!element) return null;
        const rect = element.getBoundingClientRect();
        return {
          left: Number(rect.left.toFixed(1)),
          right: Number(rect.right.toFixed(1)),
          top: Number(rect.top.toFixed(1)),
          width: Number(rect.width.toFixed(1)),
          height: Number(rect.height.toFixed(1)),
        };
      };
      const chartHeader = document.querySelector(".chart-header");
      const visibleHeaderItems = chartHeader ? Array.from(chartHeader.querySelectorAll("h2, span, button")).filter((element) => {
        const rect = element.getBoundingClientRect();
        const styles = getComputedStyle(element);
        return styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1;
      }) : [];
      const overlaps = [];
      for (let outerIndex = 0; outerIndex < visibleHeaderItems.length; outerIndex += 1) {
        for (let innerIndex = outerIndex + 1; innerIndex < visibleHeaderItems.length; innerIndex += 1) {
          const first = visibleHeaderItems[outerIndex].getBoundingClientRect();
          const second = visibleHeaderItems[innerIndex].getBoundingClientRect();
          const overlapX = Math.max(0, Math.min(first.right, second.right) - Math.max(first.left, second.left));
          const overlapY = Math.max(0, Math.min(first.bottom, second.bottom) - Math.max(first.top, second.top));
          const area = overlapX * overlapY;
          if (area > 2) {
            overlaps.push({
              first: (visibleHeaderItems[outerIndex].textContent || "").replace(/\s+/g, " ").trim().slice(0, 60),
              second: (visibleHeaderItems[innerIndex].textContent || "").replace(/\s+/g, " ").trim().slice(0, 60),
              area: Math.round(area),
            });
          }
        }
      }
      return {
        viewportWidth: window.innerWidth,
        labGrid: rectFor(".lab-grid"),
        workspace: rectFor(".workspace"),
        chartHeader: rectFor(".chart-header"),
        bottomGrid: rectFor(".bottom-grid"),
        bottomMain: rectFor(".bottom-main"),
        bottomSide: rectFor(".bottom-side"),
        overlaps,
      };
    });
    assert(report.workspace, `${label} is missing the workspace`, report);
    if (viewportName === "tablet-portrait" || viewportName === "breakpoint-761") {
      assert(report.workspace.width >= report.viewportWidth - 24, `${label} tablet portrait workspace is too narrow for the chart`, report);
    }
    assert(report.overlaps.length === 0, `${label} tablet portrait chart header controls overlap`, report.overlaps);
    assert(report.bottomMain && report.bottomSide, `${label} is missing bottom data panes`, report);
    assert(report.bottomSide.top >= report.bottomMain.top + report.bottomMain.height - 1, `${label} bottom data panes are too cramped side-by-side at this width`, report);
    assert(report.bottomMain.height >= 180 && report.bottomSide.height >= 180, `${label} bottom data panes are too short to scan comfortably`, report);
    return report;
  }

  if (viewportName !== "desktop") return null;
  const report = await page.evaluate(() => {
    const rectFor = (selector) => {
      const element = document.querySelector(selector);
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      return {
        top: Number(rect.top.toFixed(1)),
        bottom: Number(rect.bottom.toFixed(1)),
        height: Number(rect.height.toFixed(1)),
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      };
    };
    const chart = document.querySelector(".chart-workspace");
    const marketChart = document.querySelector(".market-chart");
    const footer = document.querySelector(".market-footer");
    const bottom = document.querySelector(".bottom-panel");
    const bottomGrid = document.querySelector(".bottom-grid");
    const eventLog = document.querySelector(".bottom-side .event-log");
    const chartRect = chart?.getBoundingClientRect();
    const marketRect = marketChart?.getBoundingClientRect();
    const footerRect = footer?.getBoundingClientRect();
    const bottomRect = bottom?.getBoundingClientRect();
    return {
      workspace: rectFor(".workspace"),
      chart: rectFor(".chart-workspace"),
      marketChart: rectFor(".market-chart"),
      marketFooter: rectFor(".market-footer"),
      bottom: rectFor(".bottom-panel"),
      bottomGrid: rectFor(".bottom-grid"),
      eventLog: rectFor(".bottom-side .event-log"),
      chartToBottomGap: chartRect && bottomRect ? Number((bottomRect.top - chartRect.bottom).toFixed(1)) : null,
      marketToFooterGap: marketRect && footerRect ? Number((footerRect.top - marketRect.bottom).toFixed(1)) : null,
      chartContentEscapes: Boolean(chartRect && footerRect && footerRect.bottom > chartRect.bottom + 1),
    };
  });
  assert(report.workspace && report.chart && report.bottom, `${label} is missing workspace stack elements`, report);
  assert(report.chartToBottomGap >= 4, `${label} chart and bottom data panel are visually cramped`, report);
  assert(!report.chartContentEscapes, `${label} chart content escapes underneath the bottom panel`, report);
  assert(report.marketToFooterGap >= -1, `${label} market chart overlaps the market footer`, report);
  assert(report.bottom.height >= 280, `${label} bottom data panel is too short for readable tables`, report.bottom);
  assert(report.bottomGrid.height >= 210, `${label} bottom data grid is too short for readable rows`, report.bottomGrid);
  assert(report.eventLog.height >= 136, `${label} event log is too short to scan comfortably`, report.eventLog);
  return report;
}

async function assertEventLogFits(page, label) {
  const report = await page.evaluate(() => {
    return Array.from(document.querySelectorAll(".event-log")).map((log) => {
      const table = log.querySelector(".data-table");
      const logRect = log.getBoundingClientRect();
      const tableRect = table?.getBoundingClientRect();
      const styles = getComputedStyle(log);
      const originalLeft = log.scrollLeft;
      const canScrollX = log.scrollWidth > log.clientWidth + 2;
      if (canScrollX) log.scrollLeft = Math.min(18, log.scrollWidth - log.clientWidth);
      const movedX = !canScrollX || log.scrollLeft > originalLeft;
      log.scrollLeft = originalLeft;
      return {
        className: String(log.className || ""),
        visible: styles.display !== "none" && styles.visibility !== "hidden" && logRect.width > 1 && logRect.height > 1,
        overflowX: styles.overflowX,
        canScrollX,
        movedX,
        log: {
          left: Math.round(logRect.left),
          right: Math.round(logRect.right),
          width: Math.round(logRect.width),
          clientWidth: log.clientWidth,
          scrollWidth: log.scrollWidth,
        },
        table: tableRect ? {
          left: Math.round(tableRect.left),
          right: Math.round(tableRect.right),
          width: Math.round(tableRect.width),
          scrollWidth: table.scrollWidth,
        } : null,
        overflowRight: tableRect ? Math.max(0, Math.round(tableRect.right - logRect.right)) : 0,
      };
    }).filter((entry) => entry.visible);
  });
  const clipped = report.filter((entry) => (
    entry.table &&
    entry.overflowRight > 2 &&
    (!entry.canScrollX || !["auto", "scroll"].includes(entry.overflowX) || !entry.movedX)
  ));
  assert(clipped.length === 0, `${label} event log table overflow is not handled by its scroll container`, clipped);
  return report;
}

async function assertToastPlacement(page, label) {
  const report = await page.evaluate(() => {
    const toast = document.querySelector(".toast-message");
    if (!toast) return null;
    const footer = document.querySelector(".model-footer");
    const rectFor = (element) => {
      if (!element) return null;
      const rect = element.getBoundingClientRect();
      return {
        left: Math.round(rect.left),
        top: Math.round(rect.top),
        right: Math.round(rect.right),
        bottom: Math.round(rect.bottom),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      };
    };
    const toastRect = rectFor(toast);
    const footerRect = rectFor(footer);
    const overlaps = Boolean(
      toastRect &&
      footerRect &&
      toastRect.left < footerRect.right &&
      toastRect.right > footerRect.left &&
      toastRect.top < footerRect.bottom &&
      toastRect.bottom > footerRect.top
    );
    return {
      toast: toastRect,
      footer: footerRect,
      text: (toast.textContent || "").replace(/\s+/g, " ").trim(),
      overlapsFooter: overlaps,
      viewport: { width: window.innerWidth, height: window.innerHeight },
    };
  });
  if (!report) return null;
  assert(!report.overlapsFooter, `${label} toast overlaps the bottom panel footer`, report);
  return report;
}

async function toastText(page) {
  await page.waitForSelector(".toast-message span", { timeout: 800 }).catch(() => null);
  return page.locator(".toast-message span").allInnerTexts()
    .then((values) => values.map((value) => value.replace(/\s+/g, " ").trim()).filter(Boolean).join(" | "))
    .catch(() => "");
}

async function assertToastContains(page, label, expectedParts, actionName) {
  let text = "";
  const deadline = Date.now() + 1200;
  while (Date.now() < deadline) {
    text = await toastText(page);
    if (expectedParts.some((part) => text.includes(part))) break;
    await page.waitForTimeout(60);
  }
  assert(Boolean(text), `${label} ${actionName} did not show a feedback toast`);
  const matched = expectedParts.some((part) => text.includes(part));
  assert(matched, `${label} ${actionName} feedback toast is unclear`, { text, expectedParts });
  await closeToasts(page);
  return text;
}

function keepUnexpectedValidationConsoleErrors(consoleErrors, startIndex) {
  const validationConsoleErrors = consoleErrors.splice(startIndex);
  consoleErrors.push(...validationConsoleErrors.filter((entry) => !(
    entry.type === "error" &&
    /Failed to load resource: the server responded with a status of 400 \(Bad Request\)/.test(entry.text)
  )));
}

function assertLocalizedFeedback(label, text, expectedParts, forbiddenPatterns, actionName) {
  const normalized = String(text || "").replace(/\s+/g, " ").trim();
  assert(Boolean(normalized), `${label} ${actionName} did not show user-facing feedback`);
  assert(expectedParts.some((part) => normalized.includes(part)), `${label} ${actionName} feedback is not localized precisely`, {
    text: normalized,
    expectedParts,
  });
  const forbidden = forbiddenPatterns.find((pattern) => pattern.test(normalized));
  assert(!forbidden, `${label} ${actionName} leaked a raw or wrong-locale error`, {
    text: normalized,
    forbidden: String(forbidden),
  });
  return normalized;
}

async function assertMainInteractions(page, label, viewportName, theme) {
  if (viewportName !== "desktop") return null;

  const activeIndexFor = (selector) => page.evaluate((candidateSelector) => (
    Array.from(document.querySelectorAll(candidateSelector)).findIndex((element) => element.classList.contains("active"))
  ), selector);

  const workspaceTabs = page.locator(".workspace-tabs button");
  const workspaceCount = await workspaceTabs.count();
  assert(workspaceCount === 3, `${label} has an unexpected workspace tab count`, { workspaceCount });
  const workspaceStates = [];
  const workspaceIndicesToTest = [2];
  workspaceStates.push(...await workspaceTabs.allInnerTexts());
  await closeToasts(page);
  for (const index of workspaceIndicesToTest) {
    await workspaceTabs.nth(index).click();
    await page.waitForTimeout(180);
    const activeIndex = await activeIndexFor(".workspace-tabs button");
    assert(activeIndex === index, `${label} workspace tab did not activate`, { expected: index, activeIndex });
  }
  await workspaceTabs.nth(0).click();
  await page.waitForTimeout(180);
  await closeToasts(page);

  const chartTools = page.locator(".chart-tools button");
  const toolCount = await chartTools.count();
  assert(toolCount >= 8, `${label} chart toolbar is missing expected controls`, { toolCount });
  const oneHourButton = page.locator(".chart-tools button").filter({ hasText: "1h" }).first();
  const fifteenMinuteButton = page.locator(".chart-tools button").filter({ hasText: "15m" }).first();
  assert(await oneHourButton.count(), `${label} missing 1h timeframe control`);
  assert(await fifteenMinuteButton.count(), `${label} missing 15m timeframe control`);
  await closeToasts(page);
  await oneHourButton.click();
  await page.waitForTimeout(120);
  assert(await oneHourButton.evaluate((element) => element.classList.contains("active")), `${label} 1h timeframe did not activate`);
  await assertToastContains(page, label, ["1h"], "1h timeframe");
  await fifteenMinuteButton.click();
  await page.waitForTimeout(120);
  assert(await fifteenMinuteButton.evaluate((element) => element.classList.contains("active")), `${label} 15m timeframe did not restore`);
  await assertToastContains(page, label, ["15m"], "15m timeframe");

  const indicatorButton = chartTools.nth(6);
  await indicatorButton.click();
  await page.waitForTimeout(160);
  assert(await page.locator(".indicator-strip").first().isVisible().catch(() => false), `${label} indicators did not open`);
  await assertToastContains(page, label, ["指标", "Indicators"], "indicator toggle");
  await indicatorButton.click();
  await page.waitForTimeout(160);
  assert(!(await page.locator(".indicator-strip").first().isVisible().catch(() => false)), `${label} indicators did not close`);
  await assertToastContains(page, label, ["指标", "Indicators"], "indicator close toggle");

  const expandButton = chartTools.last();
  await expandButton.click();
  await page.waitForTimeout(160);
  assert(await page.locator(".chart-workspace.expanded").first().isVisible().catch(() => false), `${label} chart did not expand`);
  await assertToastContains(page, label, ["图表", "Chart"], "chart expand");
  await expandButton.click();
  await page.waitForTimeout(160);
  assert(!(await page.locator(".chart-workspace.expanded").first().isVisible().catch(() => false)), `${label} chart did not collapse`);
  await assertToastContains(page, label, ["图表", "Chart"], "chart collapse");

  const bottomTabs = page.locator(".bottom-tabs button");
  const bottomCount = await bottomTabs.count();
  assert(bottomCount === 7, `${label} has an unexpected bottom tab count`, { bottomCount });
  const bottomStates = [];
  await closeToasts(page);
  for (let index = 0; index < bottomCount; index += 1) {
    await bottomTabs.nth(index).click();
    await page.waitForTimeout(120);
    const activeIndex = await activeIndexFor(".bottom-tabs button");
    assert(activeIndex === index, `${label} bottom tab did not activate`, { expected: index, activeIndex });
    bottomStates.push(await bottomTabs.nth(index).innerText());
  }
  await bottomTabs.nth(0).click();
  await page.waitForTimeout(120);

  const filters = page.locator(".filter-row button");
  const filterCount = await filters.count();
  assert(filterCount >= 5, `${label} event filters are missing`, { filterCount });
  const filterStates = [];
  for (let index = 0; index < filterCount; index += 1) {
    await filters.nth(index).click();
    await page.waitForTimeout(100);
    const activeIndex = await activeIndexFor(".filter-row button");
    assert(activeIndex === index, `${label} event filter did not activate`, { expected: index, activeIndex });
    filterStates.push(await filters.nth(index).innerText());
  }
  await filters.nth(0).click();
  await page.waitForTimeout(100);

  const controlButtons = page.locator(".control-actions button");
  assert(await controlButtons.count() >= 4, `${label} simulation control actions are missing`);
  const pauseButton = controlButtons.nth(0);
  const pauseBefore = (await pauseButton.innerText()).trim();
  await closeToasts(page);
  await pauseButton.click();
  await page.waitForTimeout(140);
  const pauseAfter = (await pauseButton.innerText()).trim();
  assert(pauseBefore !== pauseAfter, `${label} pause control did not toggle`, { pauseBefore, pauseAfter });
  await assertToastContains(page, label, ["暂停", "paused"], "pause control");
  await pauseButton.click();
  await page.waitForTimeout(140);
  await assertToastContains(page, label, ["恢复", "resumed"], "resume control");

  const stopButton = controlButtons.nth(1);
  const restartButton = controlButtons.nth(2);
  const aiStepButton = controlButtons.nth(3);
  const stopBefore = (await stopButton.innerText()).trim();
  await stopButton.click();
  await page.waitForTimeout(160);
  const stopAfter = (await stopButton.innerText()).trim();
  assert(stopBefore !== stopAfter, `${label} stop run control did not toggle`, { stopBefore, stopAfter });
  await assertToastContains(page, label, ["停止", "stopped"], "stop control");
  await aiStepButton.click();
  await page.waitForTimeout(160);
  await assertToastContains(page, label, ["恢复运行", "Resume the run"], "blocked AI step");
  await stopButton.click();
  await page.waitForTimeout(160);
  await assertToastContains(page, label, ["恢复", "resumed"], "resume stopped run");

  await restartButton.click();
  await page.waitForTimeout(160);
  await assertToastContains(page, label, ["重置", "reset"], "restart control");

  const settingsButton = page.locator(".log-tools .icon-row button").first();
  const exportButton = page.locator(".log-tools .icon-row button").nth(1);
  assert(await settingsButton.count(), `${label} missing event log settings button`);
  assert(await exportButton.count(), `${label} missing event export button`);

  const openLogSettingsMenu = async () => {
    if (await page.locator(".log-settings-menu").first().isVisible().catch(() => false)) return;
    await settingsButton.click();
    await page.waitForTimeout(140);
    assert(await page.locator(".log-settings-menu").first().isVisible().catch(() => false), `${label} event log settings menu did not open`);
  };

  await closeToasts(page);
  await openLogSettingsMenu();
  const logSettingItems = page.locator(".log-settings-menu button");
  assert(await logSettingItems.count() === 2, `${label} event log settings menu has unexpected item count`);

  const autoScrollBefore = await logSettingItems.nth(0).getAttribute("aria-checked");
  await logSettingItems.nth(0).click();
  await page.waitForTimeout(140);
  const autoScrollAfter = await page.locator(".log-settings-menu button").nth(0).getAttribute("aria-checked");
  assert(autoScrollBefore !== autoScrollAfter, `${label} log auto-scroll setting did not toggle`, { autoScrollBefore, autoScrollAfter });
  await assertToastContains(page, label, ["自动滚动", "auto-scroll"], "log auto-scroll setting");

  await openLogSettingsMenu();
  const compactBefore = await page.locator(".log-settings-menu button").nth(1).getAttribute("aria-checked");
  const compactClassBefore = await page.locator(".bottom-side .event-log").first().evaluate((element) => element.classList.contains("compact-log"));
  await page.locator(".log-settings-menu button").nth(1).click();
  await page.waitForTimeout(140);
  const compactAfter = await page.locator(".log-settings-menu button").nth(1).getAttribute("aria-checked");
  const compactClassAfter = await page.locator(".bottom-side .event-log").first().evaluate((element) => element.classList.contains("compact-log"));
  assert(compactBefore !== compactAfter, `${label} compact log setting did not toggle`, { compactBefore, compactAfter });
  assert(compactClassBefore !== compactClassAfter, `${label} compact log setting did not update the event log view`, { compactClassBefore, compactClassAfter });
  await assertToastContains(page, label, ["事件日志", "event rows"], "compact log setting");

  if (await page.locator(".log-settings-menu").first().isVisible().catch(() => false)) {
    await settingsButton.click();
    await page.waitForTimeout(120);
  }
  assert(!(await page.locator(".log-settings-menu").first().isVisible().catch(() => false)), `${label} event log settings menu did not close from trigger`);

  await closeToasts(page);
  await exportButton.click();
  await page.waitForTimeout(160);
  await assertToastContains(page, label, ["已导出", "exported"], "event export");

  const toasts = page.locator(".toast-message button");
  while (await toasts.first().isVisible().catch(() => false)) {
    await toasts.first().click();
    await page.waitForTimeout(80);
  }
  await page.mouse.move(1, 1);
  return { workspaceStates, bottomStates, filterStates, pauseBefore, pauseAfter, stopBefore, stopAfter, logTools: true };
}

async function assertBottomTableHorizontalScroll(page, label, viewportName) {
  const bottomTabs = page.locator(".bottom-tabs button");
  const targets = [
    { name: "performance", tabIndex: 0, frameSelector: ".performance-table-frame", tableSelector: ".performance-table" },
    { name: "positions", tabIndex: 3, frameSelector: ".positions-table-frame", tableSelector: ".positions-table" },
    { name: "orders", tabIndex: 4, frameSelector: ".orders-table-frame", tableSelector: ".orders-table" },
  ];
  assert(await bottomTabs.count() >= 5, `${label} is missing bottom table tabs`);

  const results = [];
  for (const target of targets) {
    await bottomTabs.nth(target.tabIndex).click();
    await page.waitForTimeout(180);
    await page.locator(`${target.frameSelector} .wide-table-scroll`).first().hover({ force: true });
    await page.waitForTimeout(80);

    const report = await page.evaluate(({ frameSelector, tableSelector }) => {
      const frame = document.querySelector(frameSelector);
      const scroller = frame?.querySelector(".wide-table-scroll");
      const rail = frame?.querySelector(".wide-table-rail");
      const table = frame?.querySelector(tableSelector);
      if (!frame || !scroller || !table) return { missing: true };
      const frameRect = frame.getBoundingClientRect();
      const scrollerRect = scroller.getBoundingClientRect();
      const railRect = rail?.getBoundingClientRect();
      const track = rail?.querySelector(".wide-table-track");
      const thumb = rail?.querySelector(".wide-table-thumb");
      const trackRect = track?.getBoundingClientRect();
      const thumbRect = thumb?.getBoundingClientRect();
      const nativeScrollbar = getComputedStyle(scroller, "::-webkit-scrollbar");
      const scrollerStyle = getComputedStyle(scroller);
      const beforeEdge = getComputedStyle(frame, "::before");
      const afterEdge = getComputedStyle(frame, "::after");
      return {
        missing: false,
        frameClassName: String(frame.className || ""),
        hasOverflow: scroller.scrollWidth > scroller.clientWidth + 12,
        scrollLeft: scroller.scrollLeft,
        scrollWidth: scroller.scrollWidth,
        clientWidth: scroller.clientWidth,
        railCount: frame.querySelectorAll(".wide-table-rail").length,
        overflowX: scrollerStyle.overflowX,
        overflowY: scrollerStyle.overflowY,
        scrollerClassName: String(scroller.className || ""),
        frameHeight: Math.round(frameRect.height),
        scrollerHeight: Math.round(scrollerRect.height),
        railVisible: Boolean(rail && railRect.width > 40 && railRect.height >= 3),
        railHeight: railRect ? Math.round(railRect.height) : 0,
        trackHeight: trackRect ? Number(trackRect.height.toFixed(1)) : 0,
        thumbHeight: thumbRect ? Number(thumbRect.height.toFixed(1)) : 0,
        railTop: railRect ? Math.round(railRect.top - scrollerRect.top) : null,
        railBottomInset: railRect ? Math.round(scrollerRect.bottom - railRect.bottom) : null,
        railOverlaysScroller: Boolean(railRect && railRect.top < scrollerRect.bottom - 2 && railRect.bottom <= scrollerRect.bottom + 1),
        nativeScrollbarWidth: nativeScrollbar.width,
        nativeScrollbarHeight: nativeScrollbar.height,
        leftEdgeOpacity: beforeEdge.opacity,
        rightEdgeOpacity: afterEdge.opacity,
      };
    }, target);
    assert(!report.missing, `${label} is missing the ${target.name} wide table frame`, report);
    assert(report.hasOverflow, `${label} ${target.name} table does not expose horizontal overflow for the scroll affordance`, report);
    assert(report.railVisible, `${label} ${target.name} table horizontal rail is not visible/clickable`, report);
    assert(report.trackHeight <= 4.5 && report.thumbHeight <= 4.5, `${label} ${target.name} table custom rail is thicker than the unified scrollbar style`, report);
    assert(report.railOverlaysScroller, `${label} ${target.name} table horizontal rail is reserving a second scrollbar row`, report);
    assert(report.railCount === 1, `${label} ${target.name} table has duplicate horizontal rails`, report);
    assert(report.frameClassName.split(/\s+/).includes("has-right-overflow"), `${label} ${target.name} table does not hint at hidden right-side columns`, report);
    assert(!report.frameClassName.split(/\s+/).includes("has-left-overflow"), `${label} ${target.name} table shows a left overflow hint at scroll start`, report);
    assert(Number.parseFloat(report.rightEdgeOpacity) >= 0.9, `${label} ${target.name} table right overflow hint is not visible`, report);
    assert(Number.parseFloat(report.leftEdgeOpacity) <= 0.1, `${label} ${target.name} table left overflow hint is visible at scroll start`, report);
    assert(report.overflowX === "hidden", `${label} ${target.name} table allows a native horizontal scrollbar`, report);
    assert(["auto", "scroll"].includes(report.overflowY), `${label} ${target.name} table does not preserve vertical scrolling`, report);
    assert(!report.scrollerClassName.split(/\s+/).includes("table-scroll"), `${label} ${target.name} table inherits the generic scrollbar revealer`, report);
    assert(scrollbarTooLarge({ width: report.nativeScrollbarWidth, height: report.nativeScrollbarHeight }, 0) === false, `${label} ${target.name} table shows both native and custom horizontal scrollbars`, report);

    const rail = page.locator(`${target.frameSelector} .wide-table-rail`).first();
    const box = await rail.boundingBox();
    assert(Boolean(box), `${label} ${target.name} rail has no bounding box`);
    await page.mouse.click(box.x + box.width - 8, box.y + box.height / 2);
    await page.waitForTimeout(260);
    const afterClick = await page.locator(`${target.frameSelector} .wide-table-scroll`).first().evaluate((element) => element.scrollLeft);
    assert(afterClick > report.scrollLeft + 10, `${label} ${target.name} rail click did not move the table horizontally`, { before: report.scrollLeft, afterClick });
    const afterEdge = await page.evaluate(({ frameSelector }) => {
      const frame = document.querySelector(frameSelector);
      if (!frame) return { missing: true };
      return {
        missing: false,
        frameClassName: String(frame.className || ""),
        leftEdgeOpacity: getComputedStyle(frame, "::before").opacity,
        rightEdgeOpacity: getComputedStyle(frame, "::after").opacity,
      };
    }, target);
    assert(!afterEdge.missing, `${label} ${target.name} table frame disappeared after horizontal scroll`, afterEdge);
    assert(afterEdge.frameClassName.split(/\s+/).includes("has-left-overflow"), `${label} ${target.name} table does not hint at hidden left-side columns after scroll`, afterEdge);
    assert(!afterEdge.frameClassName.split(/\s+/).includes("has-right-overflow"), `${label} ${target.name} table still hints at right-side columns after reaching the end`, afterEdge);
    assert(Number.parseFloat(afterEdge.leftEdgeOpacity) >= 0.9, `${label} ${target.name} table left overflow hint is not visible after scroll`, afterEdge);
    assert(Number.parseFloat(afterEdge.rightEdgeOpacity) <= 0.1, `${label} ${target.name} table right overflow hint remains visible after reaching the end`, afterEdge);

    await rail.focus();
    await page.keyboard.press("Home");
    await page.waitForTimeout(80);
    const afterHome = await page.locator(`${target.frameSelector} .wide-table-scroll`).first().evaluate((element) => element.scrollLeft);
    assert(afterHome <= 2, `${label} ${target.name} rail keyboard Home did not restore the horizontal position`, { afterHome });
    results.push({ name: target.name, ...report, afterClick, afterHome });
  }

  await bottomTabs.nth(0).click();
  await page.waitForTimeout(100);
  return results;
}

async function assertPaperResetDialog(page, label) {
  const bottomTabs = page.locator(".bottom-tabs button");
  await bottomTabs.nth(2).click();
  await page.waitForTimeout(120);

  const resetButton = page.locator(".paper-ledger-toolbar button").first();
  assert(await resetButton.isVisible().catch(() => false), `${label} paper reset button is missing`);

  const nativeDialogs = [];
  const dialogHandler = async (dialog) => {
    nativeDialogs.push(dialog.type());
    await dialog.dismiss().catch(() => null);
  };
  page.on("dialog", dialogHandler);

  try {
    await closeToasts(page);
    await resetButton.click();
    await page.waitForTimeout(160);
    assert(nativeDialogs.length === 0, `${label} paper reset should use an in-app confirmation dialog, not a native browser prompt`, nativeDialogs);

    const dialog = page.locator(".paper-reset-modal").first();
    assert(await dialog.isVisible().catch(() => false), `${label} paper reset dialog did not open`);
    const dialogState = await dialog.evaluate((element) => {
      const rect = element.getBoundingClientRect();
      const input = element.querySelector("input");
      const active = document.activeElement;
      return {
        title: element.querySelector("#paper-reset-title")?.textContent?.trim() || "",
        inputFocused: input === active,
        inputPlaceholder: input?.getAttribute("placeholder") || "",
        bottom: Number(rect.bottom.toFixed(1)),
        viewportHeight: window.innerHeight,
      };
    });
    assert(dialogState.title.length > 0, `${label} paper reset dialog is missing a title`, dialogState);
    assert(dialogState.inputFocused, `${label} paper reset dialog should focus the confirmation input`, dialogState);
    assert(dialogState.inputPlaceholder.includes("RESET PAPER"), `${label} paper reset dialog does not show the exact confirmation phrase`, dialogState);
    assert(dialogState.bottom <= dialogState.viewportHeight - 8, `${label} paper reset dialog is too close to the viewport bottom`, dialogState);

    await page.locator(".paper-reset-modal .danger-confirm").click();
    await page.waitForTimeout(180);
    const mismatch = await page.evaluate(() => ({
      status: document.querySelector(".paper-reset-status")?.textContent?.replace(/\s+/g, " ").trim() || "",
      toast: document.querySelector(".toast-message span")?.textContent?.replace(/\s+/g, " ").trim() || "",
      open: Boolean(document.querySelector(".paper-reset-modal")),
    }));
    assert(mismatch.open, `${label} paper reset dialog closed after an invalid confirmation phrase`, mismatch);
    assert(mismatch.status.includes("RESET PAPER") && mismatch.toast.includes("RESET PAPER"), `${label} paper reset invalid phrase feedback is unclear`, mismatch);

    await page.keyboard.press("Escape");
    await page.waitForTimeout(160);
    assert(!(await page.locator(".paper-reset-modal").first().isVisible().catch(() => false)), `${label} paper reset dialog did not close with Escape`);
    await closeToasts(page);
    return { noNativeDialogs: true, invalidPhraseFeedback: mismatch.status };
  } finally {
    page.off("dialog", dialogHandler);
    if (await page.locator(".paper-reset-modal").first().isVisible().catch(() => false)) {
      await page.locator(".paper-reset-modal .icon-close").first().click().catch(() => null);
    }
    await bottomTabs.nth(0).click().catch(() => null);
  }
}

async function closeAnyDialog(page) {
  const dialog = page.locator("[role='dialog']").first();
  if (await dialog.isVisible().catch(() => false)) {
    await page.keyboard.press("Escape");
    await page.waitForTimeout(140);
  }
}

async function closeToasts(page) {
  for (let index = 0; index < 6; index += 1) {
    const toastCount = await page.locator(".toast-message").count().catch(() => 0);
    if (toastCount === 0) break;
    const closeButton = page.locator(".toast-message button").first();
    if (await closeButton.count().catch(() => 0)) {
      await closeButton.click({ force: true }).catch(() => null);
    }
    await page.waitForFunction(() => !document.querySelector(".toast-message"), null, { timeout: 400 }).catch(() => null);
    await page.waitForTimeout(60);
  }
}

async function assertTopBarInteractions(page, label, viewportName, theme) {
  if (viewportName !== "desktop") return null;

  const activeIndexFor = (selector) => page.evaluate((candidateSelector) => (
    Array.from(document.querySelectorAll(candidateSelector)).findIndex((element) => element.classList.contains("active"))
  ), selector);

  const sourceButtons = page.locator(".source-section .segmented button");
  assert(await sourceButtons.count() === 2, `${label} has an unexpected data source button count`);
  await sourceButtons.nth(1).click();
  await page.waitForTimeout(160);
  assert(await activeIndexFor(".source-section .segmented button") === 1, `${label} OKX data source did not activate`);
  await sourceButtons.nth(0).click();
  await page.waitForTimeout(160);
  assert(await activeIndexFor(".source-section .segmented button") === 0, `${label} Binance data source did not restore`);

  const modeButtons = page.locator(".mode-section .segmented button");
  assert(await modeButtons.count() === 3, `${label} has an unexpected mode button count`);
  const modeStates = [];
  for (let index = 1; index < 3; index += 1) {
    await modeButtons.nth(index).click();
    await page.waitForTimeout(180);
    const activeIndex = await activeIndexFor(".mode-section .segmented button");
    assert(activeIndex === index, `${label} mode button did not activate`, { expected: index, activeIndex });
    modeStates.push(await modeButtons.nth(index).innerText());
    await closeAnyDialog(page);
  }
  await modeButtons.nth(0).click();
  await page.waitForTimeout(180);
  assert(await activeIndexFor(".mode-section .segmented button") === 0, `${label} Shadow mode did not restore`);

  const dialogTriggers = [
    { name: "top-strategy", trigger: ".strategy-section .select-button" },
    { name: "top-model", trigger: ".model-config-button" },
    { name: "top-guard", trigger: ".guard-link" },
    { name: "top-vault", trigger: ".connection-vault-link" },
  ];
  const openedDialogs = [];
  for (const item of dialogTriggers) {
    await closeAnyDialog(page);
    await page.locator(item.trigger).first().click();
    await page.waitForTimeout(220);
    assert(await page.locator("[role='dialog']").first().isVisible().catch(() => false), `${label} ${item.name} trigger did not open a dialog`);
    openedDialogs.push(item.name);
    await closeAnyDialog(page);
    assert(!(await page.locator("[role='dialog']").first().isVisible().catch(() => false)), `${label} ${item.name} dialog did not close`);
  }

  const stopAll = page.locator(".stop-all").first();
  assert(await stopAll.count(), `${label} missing stop-all button`);
  const stopTextBefore = (await stopAll.innerText()).replace(/\s+/g, " ").trim();
  const stopPressedBefore = await stopAll.getAttribute("aria-pressed");
  await closeToasts(page);
  await stopAll.click();
  await page.waitForFunction((initialPressed) => {
    const button = document.querySelector(".stop-all");
    return button &&
      button.getAttribute("aria-busy") !== "true" &&
      button.getAttribute("aria-pressed") !== initialPressed;
  }, stopPressedBefore, { timeout: 5000 });
  const stopTextAfter = (await stopAll.innerText()).replace(/\s+/g, " ").trim();
  assert(stopTextBefore !== stopTextAfter, `${label} stop-all button did not toggle`, { stopTextBefore, stopTextAfter });
  await stopAll.click();
  await page.waitForFunction((initialPressed) => {
    const button = document.querySelector(".stop-all");
    return button &&
      button.getAttribute("aria-busy") !== "true" &&
      button.getAttribute("aria-pressed") === initialPressed;
  }, stopPressedBefore, { timeout: 5000 });
  const stopTextRestored = (await stopAll.innerText()).replace(/\s+/g, " ").trim();
  assert(stopTextRestored === stopTextBefore, `${label} stop-all button did not restore`, { stopTextBefore, stopTextRestored });

  await closeToasts(page);
  await page.mouse.move(1, 1);
  return { sourceStates: ["OKX", "Binance"], modeStates, openedDialogs, stopTextBefore, stopTextAfter };
}

async function assertHeaderSwitchers(page, label, expectedTheme, expectedLocale) {
  const initialState = await page.evaluate(() => ({
    theme: document.documentElement.dataset.theme,
    locale: document.documentElement.lang,
    storedTheme: localStorage.getItem("ccvar.theme"),
    storedLocale: localStorage.getItem("ccvar.locale"),
    themeText: document.querySelector(".theme-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
    languageText: document.querySelector(".language-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
  }));
  assert(initialState.theme === expectedTheme, `${label} did not boot in the expected theme`, initialState);
  assert(initialState.locale === expectedLocale, `${label} did not boot in the expected locale`, initialState);
  assert(initialState.storedTheme === expectedTheme, `${label} did not persist the expected theme`, initialState);
  assert(initialState.storedLocale === expectedLocale, `${label} did not persist the expected locale`, initialState);

  const alternateTheme = expectedTheme === "dark" ? "light" : "dark";
  const alternateLocale = expectedLocale === "zh-CN" ? "en-US" : "zh-CN";
  const labelForLocale = (locale) => (locale === "zh-CN" ? "中文" : "English");

  const languageTrigger = page.locator(".language-trigger").first();
  assert(await languageTrigger.count(), `${label} missing language switcher`);
  await languageTrigger.click();
  await page.waitForTimeout(120);
  assert(await page.locator(".language-menu").first().isVisible().catch(() => false), `${label} language menu did not open`);
  const alternateOption = page.locator(".language-menu button").filter({ hasText: labelForLocale(alternateLocale) }).first();
  assert(await alternateOption.count(), `${label} missing language option for ${alternateLocale}`);
  await alternateOption.click();
  await page.waitForFunction((locale) => (
    document.documentElement.lang === locale &&
    localStorage.getItem("ccvar.locale") === locale &&
    (document.querySelector(".language-trigger")?.textContent || "").includes(locale === "zh-CN" ? "中文" : "English")
  ), alternateLocale);
  const switchedLocale = await page.evaluate(() => ({
    locale: document.documentElement.lang,
    storedLocale: localStorage.getItem("ccvar.locale"),
    triggerText: document.querySelector(".language-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
  }));
  assert(switchedLocale.locale === alternateLocale && switchedLocale.storedLocale === alternateLocale, `${label} language switch did not apply`, switchedLocale);

  await languageTrigger.click();
  await page.waitForTimeout(120);
  const restoreOption = page.locator(".language-menu button").filter({ hasText: labelForLocale(expectedLocale) }).first();
  assert(await restoreOption.count(), `${label} missing restore language option for ${expectedLocale}`);
  await restoreOption.click();
  await page.waitForFunction((locale) => (
    document.documentElement.lang === locale &&
    localStorage.getItem("ccvar.locale") === locale &&
    (document.querySelector(".language-trigger")?.textContent || "").includes(locale === "zh-CN" ? "中文" : "English")
  ), expectedLocale);

  const themeTrigger = page.locator(".theme-trigger").first();
  assert(await themeTrigger.count(), `${label} missing theme switcher`);
  await themeTrigger.click();
  await page.waitForFunction((theme) => (
    document.documentElement.dataset.theme === theme &&
    localStorage.getItem("ccvar.theme") === theme
  ), alternateTheme);
  const switchedTheme = await page.evaluate(() => ({
    theme: document.documentElement.dataset.theme,
    storedTheme: localStorage.getItem("ccvar.theme"),
    triggerText: document.querySelector(".theme-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
  }));
  assert(switchedTheme.theme === alternateTheme && switchedTheme.storedTheme === alternateTheme, `${label} theme switch did not apply`, switchedTheme);

  await themeTrigger.click();
  await page.waitForFunction((theme) => (
    document.documentElement.dataset.theme === theme &&
    localStorage.getItem("ccvar.theme") === theme
  ), expectedTheme);
  const restoredState = await page.evaluate(() => ({
    theme: document.documentElement.dataset.theme,
    locale: document.documentElement.lang,
    storedTheme: localStorage.getItem("ccvar.theme"),
    storedLocale: localStorage.getItem("ccvar.locale"),
    themeText: document.querySelector(".theme-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
    languageText: document.querySelector(".language-trigger")?.textContent?.replace(/\s+/g, " ").trim() || "",
  }));
  assert(restoredState.theme === expectedTheme && restoredState.locale === expectedLocale, `${label} header switchers did not restore cleanly`, restoredState);
  assert(restoredState.storedTheme === expectedTheme && restoredState.storedLocale === expectedLocale, `${label} header switcher persistence did not restore cleanly`, restoredState);

  const toastClose = page.locator(".toast-message button").first();
  if (await toastClose.isVisible().catch(() => false)) {
    await toastClose.click();
    await page.waitForTimeout(120);
  }
  return { switchedLocale, switchedTheme, restoredState };
}

async function assertMobilePrimaryContent(page, label) {
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(80);
  const report = await page.evaluate(() => {
    const brand = document.querySelector(".brand-block");
    const title = document.querySelector(".brand-block h1");
    if (!brand) return { missing: true };
    const rect = brand.getBoundingClientRect();
    const titleRect = title?.getBoundingClientRect();
    const titleStyle = title ? getComputedStyle(title) : null;
    const titleCenterX = titleRect ? titleRect.left + titleRect.width / 2 : 0;
    const titleCenterY = titleRect ? titleRect.top + titleRect.height / 2 : 0;
    const topElement = titleRect ? document.elementFromPoint(titleCenterX, titleCenterY) : null;
    return {
      missing: false,
      top: Math.round(rect.top),
      bottom: Math.round(rect.bottom),
      viewportHeight: window.innerHeight,
      maxTop: Math.round(Math.min(340, window.innerHeight * 0.42)),
      title: titleRect ? {
        top: Math.round(titleRect.top),
        bottom: Math.round(titleRect.bottom),
        width: Math.round(titleRect.width),
        height: Math.round(titleRect.height),
        visible: titleStyle?.display !== "none" && titleStyle?.visibility !== "hidden" && titleRect.width > 44 && titleRect.height > 9,
        topElementTag: topElement?.tagName || "",
        topElementClass: typeof topElement?.className === "string" ? topElement.className : "",
      } : null,
    };
  });
  assert(!report.missing, `${label} is missing the brand block`, report);
  assert(report.top <= report.maxTop, `${label} pushes primary content too far below the fold`, report);
  assert(report.title?.visible, `${label} mobile brand title is not visible enough`, report);
  assert(report.title?.topElementTag === "H1" || String(report.title?.topElementClass || "").includes("brand"), `${label} mobile brand title is visually covered`, report);
}

async function controlReport(page, scopeSelector = null) {
  return page.evaluate(({ scopeSelector: selector }) => {
    const root = selector ? document.querySelector(selector) : document;
    if (!root) return [];
    const controls = Array.from(root.querySelectorAll("button,input,select,textarea,[role='button'],[role='tab'],[role='switch'],[aria-haspopup]"));
    return controls.map((element, index) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      const text = (element.innerText || element.value || element.getAttribute("aria-label") || element.getAttribute("title") || "").replace(/\s+/g, " ").trim();
      const labelText = element.labels?.length ? Array.from(element.labels).map((label) => (label.innerText || "").replace(/\s+/g, " ").trim()).join(" ") : "";
      const accessibleName = text || element.getAttribute("aria-label") || element.getAttribute("title") || element.getAttribute("aria-labelledby") || labelText;
      const inputType = element.getAttribute("type") || "";
      const className = typeof element.className === "string" ? element.className : "";
      const tiny = rect.width < 20 || rect.height < 20;
      const labelBackedSmall = tiny && element.tagName === "INPUT" && ["checkbox", "range"].includes(inputType) && Boolean(labelText || element.getAttribute("aria-label"));
      const compactBrandSwitcher = tiny &&
        Boolean(element.closest(".brand-meta-row")) &&
        (className.split(/\s+/).includes("theme-trigger") || className.split(/\s+/).includes("language-trigger")) &&
        rect.height >= 18 &&
        rect.width >= 36;
      return {
        index,
        tag: element.tagName,
        type: inputType,
        className: className.slice(0, 120),
        text: text.slice(0, 100),
        ariaLabel: (element.getAttribute("aria-label") || "").slice(0, 100),
        labelText: labelText.slice(0, 100),
        role: element.getAttribute("role") || "",
        ariaPressed: element.getAttribute("aria-pressed"),
        ariaSelected: element.getAttribute("aria-selected"),
        ariaChecked: element.getAttribute("aria-checked"),
        ariaCurrent: element.getAttribute("aria-current"),
        disabled: Boolean(element.disabled || element.getAttribute("aria-disabled") === "true"),
        visible: styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1,
        rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) },
        missingName: !accessibleName,
        tinyAction: tiny && !labelBackedSmall && !compactBrandSwitcher,
        activeWithoutState: element.tagName === "BUTTON" &&
          className.split(/\s+/).includes("active") &&
          !element.getAttribute("aria-pressed") &&
          !element.getAttribute("aria-selected") &&
          !element.getAttribute("aria-checked") &&
          !element.getAttribute("aria-current") &&
          element.getAttribute("role") !== "menuitemcheckbox",
      };
    }).filter((entry) => entry.visible);
  }, { scopeSelector });
}

async function assertControls(page, label, scopeSelector = null) {
  const controls = await controlReport(page, scopeSelector);
  const missingNames = controls.filter((entry) => entry.missingName);
  const tinyActions = controls.filter((entry) => !entry.disabled && entry.tinyAction);
  const activeWithoutState = controls.filter((entry) => !entry.disabled && entry.activeWithoutState);
  assert(missingNames.length === 0, `${label} has controls without accessible names`, missingNames);
  assert(tinyActions.length === 0, `${label} has enabled action controls below 20px`, tinyActions);
  assert(activeWithoutState.length === 0, `${label} has active buttons without an accessible state`, activeWithoutState);
  return controls.length;
}

async function assertNumberInputChrome(page, label, scopeSelector = null) {
  const report = await page.evaluate(({ scopeSelector: selector }) => {
    const root = selector ? document.querySelector(selector) : document;
    if (!root) return [];
    return Array.from(root.querySelectorAll('input[type="number"]')).map((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      return {
        value: element.value,
        className: String(element.className || ""),
        visible: styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1,
        appearance: styles.appearance,
        webkitAppearance: styles.webkitAppearance,
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      };
    }).filter((entry) => entry.visible);
  }, { scopeSelector });
  const nativeNumberInputs = report.filter((entry) => entry.appearance === "auto" || entry.webkitAppearance === "auto");
  assert(nativeNumberInputs.length === 0, `${label} has native browser number spinners`, nativeNumberInputs);
  return report;
}

async function assertPopupGeometry(page, popupSelector, label) {
  const report = await page.locator(popupSelector).first().evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const textClips = Array.from(element.querySelectorAll("button,span,strong,small")).map((child) => {
      const childRect = child.getBoundingClientRect();
      const styles = getComputedStyle(child);
      const text = (child.textContent || "").replace(/\s+/g, " ").trim();
      return {
        tag: child.tagName,
        className: String(child.className || ""),
        text,
        visible: styles.display !== "none" && styles.visibility !== "hidden" && childRect.width > 1 && childRect.height > 1,
        clientWidth: child.clientWidth,
        scrollWidth: child.scrollWidth,
        clientHeight: child.clientHeight,
        scrollHeight: child.scrollHeight,
      };
    }).filter((entry) => (
      entry.visible &&
      entry.text &&
      (entry.scrollWidth > entry.clientWidth + 1 || entry.scrollHeight > entry.clientHeight + 1)
    ));
    return {
      left: Math.round(rect.left),
      top: Math.round(rect.top),
      right: Math.round(rect.right),
      bottom: Math.round(rect.bottom),
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      viewportWidth: window.innerWidth,
      viewportHeight: window.innerHeight,
      textClips,
    };
  });
  assert(report.left >= -2 && report.top >= -2 && report.right <= report.viewportWidth + 2 && report.bottom <= report.viewportHeight + 2, `${label} popup is outside viewport`, report);
  assert(report.textClips.length === 0, `${label} popup has clipped text`, report.textClips);
  return report;
}

async function assertMenus(page, label) {
  const menus = [
    { name: "language", trigger: ".language-trigger", popup: ".language-menu" },
    { name: "data-mode", trigger: ".data-select > button", popup: ".data-menu" },
    { name: "jump", trigger: ".jump-select > button", popup: ".jump-menu" },
    { name: "log-settings", trigger: ".log-tools .icon-row button:first-child", popup: ".log-settings-menu" },
  ];
  const expectsSolidLightPopup = label.includes("-light-");
  for (const menu of menus) {
    await page.keyboard.press("Escape").catch(() => {});
    const trigger = page.locator(menu.trigger).first();
    assert(await trigger.count(), `missing menu trigger: ${menu.name}`);
    await trigger.click();
    await page.waitForTimeout(160);
    assert(await page.locator(menu.popup).first().isVisible().catch(() => false), `menu did not open: ${menu.name}`);
    await assertPopupGeometry(page, menu.popup, `${label} ${menu.name}`);
    if (expectsSolidLightPopup) {
      const popupPaint = await page.locator(menu.popup).first().evaluate((element) => {
        const backgroundColor = getComputedStyle(element).backgroundColor;
        const alphaMatch = backgroundColor.match(/rgba?\(([^)]+)\)/i);
        const parts = alphaMatch ? alphaMatch[1].split(",").map((part) => part.trim()) : [];
        return {
          backgroundColor,
          alpha: parts.length >= 4 ? Number.parseFloat(parts[3]) : 1,
        };
      });
      assert(popupPaint.alpha >= 0.995, `${label} ${menu.name} popup background is too translucent`, popupPaint);
    }
    await screenshot(page, `menus/${label}-${menu.name}`);
    await page.locator(".chart-workspace").first().click({ position: { x: 12, y: 12 } }).catch(async () => {
      await page.mouse.click(720, 220);
    });
    await page.waitForTimeout(120);
    assert(!(await page.locator(menu.popup).first().isVisible().catch(() => false)), `menu did not close on outside click: ${menu.name}`);
    await trigger.click();
    await page.waitForTimeout(120);
    assert(await page.locator(menu.popup).first().isVisible().catch(() => false), `menu did not reopen after outside dismissal: ${menu.name}`);
    await page.keyboard.press("Escape");
    await page.waitForTimeout(120);
    assert(!(await page.locator(menu.popup).first().isVisible().catch(() => false)), `menu did not close on Escape: ${menu.name}`);

    await trigger.focus();
    await page.keyboard.press("ArrowDown");
    await page.waitForTimeout(160);
    assert(await page.locator(menu.popup).first().isVisible().catch(() => false), `menu did not open from ArrowDown: ${menu.name}`);
    const keyboardOpenState = await page.evaluate(({ triggerSelector, popupSelector }) => {
      const triggerElement = document.querySelector(triggerSelector);
      const popupElement = document.querySelector(popupSelector);
      const items = Array.from(popupElement?.querySelectorAll('[role="menuitem"], [role="menuitemcheckbox"], [role="option"]') || []);
      return {
        activeInsidePopup: Boolean(popupElement?.contains(document.activeElement)),
        activeIndex: items.indexOf(document.activeElement),
        itemCount: items.length,
        triggerFocused: document.activeElement === triggerElement,
      };
    }, { triggerSelector: menu.trigger, popupSelector: menu.popup });
    assert(keyboardOpenState.activeInsidePopup && keyboardOpenState.activeIndex === 0, `menu did not focus its first item after ArrowDown: ${menu.name}`, keyboardOpenState);
    if (keyboardOpenState.itemCount > 1) {
      await page.keyboard.press("ArrowDown");
      await page.waitForTimeout(80);
      const arrowMoveState = await page.evaluate((popupSelector) => {
        const popupElement = document.querySelector(popupSelector);
        const items = Array.from(popupElement?.querySelectorAll('[role="menuitem"], [role="menuitemcheckbox"], [role="option"]') || []);
        return {
          activeInsidePopup: Boolean(popupElement?.contains(document.activeElement)),
          activeIndex: items.indexOf(document.activeElement),
        };
      }, menu.popup);
      assert(arrowMoveState.activeInsidePopup && arrowMoveState.activeIndex === 1, `menu ArrowDown did not move focus to the next item: ${menu.name}`, arrowMoveState);
    }
    await page.keyboard.press("Escape");
    await page.waitForTimeout(140);
    const keyboardCloseState = await page.evaluate(({ triggerSelector, popupSelector }) => ({
      popupOpen: Boolean(document.querySelector(popupSelector)),
      triggerFocused: document.activeElement === document.querySelector(triggerSelector),
      activeText: document.activeElement?.textContent?.replace(/\s+/g, " ").trim() || "",
    }), { triggerSelector: menu.trigger, popupSelector: menu.popup });
    assert(!keyboardCloseState.popupOpen && keyboardCloseState.triggerFocused, `menu Escape did not close and restore focus to trigger: ${menu.name}`, keyboardCloseState);
  }
}

async function assertDialog(page, dialog, viewportName, label, consoleErrors = []) {
  await page.keyboard.press("Escape").catch(() => {});
  const trigger = page.locator(dialog.trigger).first();
  assert(await trigger.count(), `missing dialog trigger: ${dialog.name}`);
  const dialogElementLocator = page.locator("[role='dialog']").first();
  const openDialog = async (phase) => {
    await trigger.click();
    await page.waitForTimeout(250);
    assert(await dialogElementLocator.isVisible().catch(() => false), `dialog did not open: ${dialog.name} (${phase})`);
    const focusState = await page.evaluate(() => {
      const dialogElement = document.querySelector("[role='dialog']");
      const active = document.activeElement;
      return {
        insideDialog: Boolean(dialogElement && active && dialogElement.contains(active)),
        tag: active?.tagName || "",
        className: typeof active?.className === "string" ? active.className : "",
        ariaLabel: active?.getAttribute?.("aria-label") || "",
      };
    });
    assert(focusState.insideDialog, `dialog did not move focus inside on open: ${dialog.name} (${phase})`, focusState);
    return dialogElementLocator;
  };
  const assertDialogClosed = async (phase) => {
    await page.waitForTimeout(140);
    assert(!(await dialogElementLocator.isVisible().catch(() => false)), `dialog did not close via ${phase}: ${dialog.name}`);
  };
  const assertFocusRestored = async (phase) => {
    await page.waitForTimeout(120);
    const focusState = await trigger.evaluate((element) => {
      const active = document.activeElement;
      return {
        restored: active === element,
        tag: active?.tagName || "",
        className: typeof active?.className === "string" ? active.className : "",
        text: (active?.textContent || "").replace(/\s+/g, " ").trim().slice(0, 80),
      };
    });
    assert(focusState.restored, `dialog did not restore focus after ${phase}: ${dialog.name}`, focusState);
  };
  const assertFocusContained = async (phase) => {
    const focusState = await page.evaluate(() => {
      const dialogElement = document.querySelector("[role='dialog']");
      const active = document.activeElement;
      return {
        insideDialog: Boolean(dialogElement && active && dialogElement.contains(active)),
        tag: active?.tagName || "",
        className: typeof active?.className === "string" ? active.className : "",
        text: (active?.textContent || active?.getAttribute?.("aria-label") || "").replace(/\s+/g, " ").trim().slice(0, 100),
      };
    });
    assert(focusState.insideDialog, `dialog focus escaped after ${phase}: ${dialog.name}`, focusState);
  };
  const assertFocusTrap = async () => {
    for (let index = 0; index < 18; index += 1) {
      await page.keyboard.press("Tab");
      await page.waitForTimeout(15);
      await assertFocusContained(`Tab ${index + 1}`);
    }
    for (let index = 0; index < 8; index += 1) {
      await page.keyboard.press("Shift+Tab");
      await page.waitForTimeout(15);
      await assertFocusContained(`Shift+Tab ${index + 1}`);
    }
  };

  let dialogElement = await openDialog("close-button check");
  await dialogElement.click({ position: { x: 18, y: 18 } });
  await page.waitForTimeout(100);
  assert(await dialogElementLocator.isVisible().catch(() => false), `dialog closed when clicking inside content: ${dialog.name}`);
  const closeButton = dialogElement.locator(".icon-close").first();
  assert(await closeButton.count(), `dialog is missing a close button: ${dialog.name}`);
  await closeButton.click();
  await assertDialogClosed("close button");
  await assertFocusRestored("close button");

  dialogElement = await openDialog("backdrop check");
  const backdrop = page.locator(".modal-backdrop").first();
  assert(await backdrop.count(), `dialog is missing a backdrop: ${dialog.name}`);
  await backdrop.click({ position: { x: 2, y: 2 } });
  await assertDialogClosed("backdrop click");
  await assertFocusRestored("backdrop click");

  dialogElement = await openDialog("validation");
  const rect = await dialogElement.evaluate((element) => {
    const r = element.getBoundingClientRect();
    return { left: r.left, top: r.top, right: r.right, bottom: r.bottom, width: r.width, height: r.height, viewportWidth: innerWidth, viewportHeight: innerHeight };
  });
  assert(rect.left >= -2 && rect.top >= -2 && rect.right <= rect.viewportWidth + 2 && rect.bottom <= rect.viewportHeight + 2, `dialog is outside viewport: ${dialog.name}`, rect);
  await assertFocusTrap();

  await assertControls(page, `${dialog.name} dialog`, "[role='dialog']");
  await assertNumberInputChrome(page, `${label} ${dialog.name} dialog`, "[role='dialog']");
  const headerLayout = await dialogElement.evaluate((element) => {
    const header = element.querySelector(".credential-modal-header");
    if (!header) return { missing: true };
    const titleBlock = header.querySelector(":scope > div:first-child");
    const actionBlock = header.querySelector(".modal-header-actions") || header.querySelector(".icon-close");
    const closeButton = header.querySelector(".icon-close");
    const rectFor = (node) => {
      if (!node) return null;
      const rect = node.getBoundingClientRect();
      return {
        left: Math.round(rect.left),
        top: Math.round(rect.top),
        right: Math.round(rect.right),
        bottom: Math.round(rect.bottom),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      };
    };
    const backgroundColor = closeButton ? getComputedStyle(closeButton).backgroundColor : "";
    const alphaMatch = backgroundColor.match(/rgba?\(([^)]+)\)/i);
    const parts = alphaMatch ? alphaMatch[1].split(",").map((part) => part.trim()) : [];
    return {
      missing: !titleBlock || !actionBlock || !closeButton,
      header: rectFor(header),
      titleBlock: rectFor(titleBlock),
      actionBlock: rectFor(actionBlock),
      closeButton: rectFor(closeButton),
      closeBackgroundColor: backgroundColor,
      closeAlpha: parts.length >= 4 ? Number.parseFloat(parts[3]) : 1,
    };
  });
  assert(!headerLayout.missing, `${dialog.name} dialog header is missing title or actions`, headerLayout);
  assert(headerLayout.closeButton.width >= 28 && headerLayout.closeButton.height >= 28, `${dialog.name} dialog close button is too small`, headerLayout);
  assert(headerLayout.titleBlock.right <= headerLayout.actionBlock.left - 2 || headerLayout.titleBlock.bottom <= headerLayout.actionBlock.top || headerLayout.titleBlock.top >= headerLayout.actionBlock.bottom, `${dialog.name} dialog header title overlaps actions`, headerLayout);
  if (label.includes("-light-")) {
    assert(headerLayout.closeAlpha > 0.2, `${label} ${dialog.name} dialog close button is visually floating on light theme`, headerLayout);
  }
  await dialogElement.evaluate((element) => {
    [element, ...Array.from(element.querySelectorAll("*"))].forEach((candidate) => {
      candidate.scrollTop = 0;
      candidate.scrollLeft = 0;
    });
  });
  await page.waitForTimeout(60);
  await screenshot(page, `modals/${label}-${dialog.name}`);

  const scrollReport = await dialogElement.evaluate((element) => {
    const candidates = [element, ...Array.from(element.querySelectorAll("*"))];
    return candidates.map((candidate) => {
      const rect = candidate.getBoundingClientRect();
      const styles = getComputedStyle(candidate);
      const canScrollY = /^(auto|scroll|overlay)$/.test(styles.overflowY);
      const canScrollX = /^(auto|scroll|overlay)$/.test(styles.overflowX);
      const scrollableY = canScrollY && candidate.scrollHeight > candidate.clientHeight + 4;
      const scrollableX = canScrollX && candidate.scrollWidth > candidate.clientWidth + 4;
      if (!scrollableY && !scrollableX) return null;
      const before = { top: candidate.scrollTop, left: candidate.scrollLeft };
      candidate.scrollTop = candidate.scrollHeight;
      candidate.scrollLeft = candidate.scrollWidth;
      return {
        className: String(candidate.className || candidate.tagName),
        visible: styles.display !== "none" && styles.visibility !== "hidden" && rect.width > 1 && rect.height > 1,
        clientWidth: candidate.clientWidth,
        scrollWidth: candidate.scrollWidth,
        clientHeight: candidate.clientHeight,
        scrollHeight: candidate.scrollHeight,
        before,
        after: { top: candidate.scrollTop, left: candidate.scrollLeft },
        reachedBottom: !scrollableY || candidate.scrollTop + candidate.clientHeight >= candidate.scrollHeight - 2,
        reachedRight: !scrollableX || candidate.scrollLeft + candidate.clientWidth >= candidate.scrollWidth - 2,
      };
    }).filter((entry) => entry && entry.visible);
  });
  const unreachableScroll = scrollReport.filter((entry) => !entry.reachedBottom || !entry.reachedRight);
  assert(unreachableScroll.length === 0, `${dialog.name} dialog has unreachable scrollable content`, unreachableScroll);
  if (scrollReport.length > 0) {
    await page.waitForTimeout(80);
    await screenshot(page, `modals/${label}-${dialog.name}-bottom`);
    await dialogElement.evaluate((element) => {
      [element, ...Array.from(element.querySelectorAll("*"))].forEach((candidate) => {
        candidate.scrollTop = 0;
        candidate.scrollLeft = 0;
      });
    });
  }

  if (dialog.name === "vault" && viewportName === "desktop") {
    const vaultLayout = await dialogElement.evaluate((element) => {
      const grid = element.querySelector(".credential-modal-grid");
      const form = element.querySelector(".credential-form");
      const list = element.querySelector(".credential-list");
      const info = (node) => {
        if (!node) return null;
        const rect = node.getBoundingClientRect();
        const styles = getComputedStyle(node);
        return {
          top: Math.round(rect.top),
          bottom: Math.round(rect.bottom),
          height: Math.round(rect.height),
          overflowY: styles.overflowY,
          backgroundColor: styles.backgroundColor,
          clientHeight: node.clientHeight,
          scrollHeight: node.scrollHeight,
        };
      };
      const gridInfo = info(grid);
      const formInfo = info(form);
      const listInfo = info(list);
      return {
        missing: !grid || !form || !list,
        grid: gridInfo,
        form: formInfo,
        list: listInfo,
        footerGap: gridInfo && formInfo && listInfo
          ? Math.round(gridInfo.bottom - Math.max(formInfo.bottom, listInfo.bottom))
          : null,
      };
    });
    assert(!vaultLayout.missing, `${label} vault dialog is missing its two-column layout`, vaultLayout);
    assert(vaultLayout.grid.overflowY === "hidden", `${label} vault dialog outer grid should not scroll on desktop`, vaultLayout);
    assert(vaultLayout.grid.scrollHeight <= vaultLayout.grid.clientHeight + 2, `${label} vault dialog outer grid has extra scroll height`, vaultLayout);
    assert(/^(auto|scroll|overlay)$/.test(vaultLayout.form.overflowY), `${label} vault form column is not internally scrollable`, vaultLayout);
    assert(/^(auto|scroll|overlay)$/.test(vaultLayout.list.overflowY), `${label} vault saved-key column is not internally scrollable`, vaultLayout);
    assert(vaultLayout.grid.backgroundColor === vaultLayout.form.backgroundColor && vaultLayout.grid.backgroundColor === vaultLayout.list.backgroundColor, `${label} vault dialog column backgrounds do not match`, vaultLayout);
    assert(vaultLayout.footerGap !== null && vaultLayout.footerGap <= 2, `${label} vault dialog exposes a bottom background band`, vaultLayout);
  }

  const clippedCards = await page.evaluate(() => {
    const selector = ".ai-active-card,.ai-route-map,.ai-config-summary,.strategy-summary";
    return Array.from(document.querySelectorAll(selector)).map((element) => {
      const rect = element.getBoundingClientRect();
      return {
        className: String(element.className || element.tagName),
        text: (element.textContent || "").replace(/\s+/g, " ").trim().slice(0, 120),
        visible: rect.width > 1 && rect.height > 1 && getComputedStyle(element).visibility !== "hidden",
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      };
    }).filter((entry) => entry.visible && entry.scrollHeight > entry.clientHeight + 1);
  });
  assert(clippedCards.length === 0, `${dialog.name} dialog has clipped summary cards`, clippedCards);

  const clippedAuditRows = await page.evaluate(() => {
    return Array.from(document.querySelectorAll(".audit-rows")).map((element) => {
      const rect = element.getBoundingClientRect();
      const styles = getComputedStyle(element);
      return {
        className: String(element.className || "audit-rows"),
        visible: rect.width > 1 && rect.height > 1 && styles.display !== "none" && styles.visibility !== "hidden",
        overflowY: styles.overflowY,
        clientHeight: element.clientHeight,
        scrollHeight: element.scrollHeight,
      };
    }).filter((entry) => entry.visible && entry.overflowY === "hidden" && entry.scrollHeight > entry.clientHeight + 1);
  });
  assert(clippedAuditRows.length === 0, `${dialog.name} dialog clips audit rows`, clippedAuditRows);

  if (viewportName === "mobile") {
    const overlaps = await page.evaluate(() => {
      const grids = Array.from(document.querySelectorAll(".credential-modal-grid,.live-guard-grid"));
      return grids.flatMap((grid) => {
        const children = Array.from(grid.children).filter((child) => {
          const rect = child.getBoundingClientRect();
          return rect.width > 1 && rect.height > 1;
        });
        return children.slice(1).map((child, index) => {
          const previous = children[index].getBoundingClientRect();
          const current = child.getBoundingClientRect();
          const gap = current.top - previous.bottom;
          return {
            grid: String(grid.className),
            previous: String(children[index].className),
            current: String(child.className),
            gap: Math.round(gap),
          };
        }).filter((entry) => entry.gap < -1);
      });
    });
    assert(overlaps.length === 0, `mobile dialog sections overlap: ${dialog.name}`, overlaps);

    const bottomGaps = await page.evaluate(() => {
      const dialogElement = document.querySelector("[role='dialog']");
      if (!dialogElement) return [];
      const candidates = Array.from(dialogElement.querySelectorAll(".credential-modal-grid,.live-guard-grid,.strategy-profile-body,.ai-config-body"));
      return candidates.map((container) => {
        const styles = getComputedStyle(container);
        const rect = container.getBoundingClientRect();
        const scrollable = /^(auto|scroll|overlay)$/.test(styles.overflowY) && container.scrollHeight > container.clientHeight + 4;
        if (!scrollable || styles.display === "none" || styles.visibility === "hidden" || rect.width < 2 || rect.height < 2) return null;
        container.scrollTop = container.scrollHeight;
        const childRects = Array.from(container.children).map((child) => {
          const childRect = child.getBoundingClientRect();
          const childStyles = getComputedStyle(child);
          return childStyles.display !== "none" && childStyles.visibility !== "hidden" && childRect.width > 1 && childRect.height > 1
            ? childRect
            : null;
        }).filter(Boolean);
        const contentBottom = childRects.length ? Math.max(...childRects.map((childRect) => childRect.bottom)) : rect.top;
        return {
          className: String(container.className || container.tagName),
          gap: Math.round(rect.bottom - contentBottom),
          paddingBottom: Math.round(Number.parseFloat(styles.paddingBottom) || 0),
          containerBottom: Math.round(rect.bottom),
          contentBottom: Math.round(contentBottom),
        };
      }).filter(Boolean);
    });
    const crampedBottoms = bottomGaps.filter((entry) => entry.gap < 8);
    assert(crampedBottoms.length === 0, `mobile dialog scroll bottom lacks breathing room: ${dialog.name}`, crampedBottoms);

    if (dialog.name === "vault") {
      const emptyState = await page.evaluate(() => {
        const element = document.querySelector(".credential-list .empty-vault");
        if (!element) return { missing: true };
        const rect = element.getBoundingClientRect();
        const childRects = Array.from(element.children).map((child) => child.getBoundingClientRect());
        return {
          missing: false,
          visible: rect.top < window.innerHeight && rect.bottom > 0,
          top: Math.round(rect.top),
          bottom: Math.round(rect.bottom),
          viewportHeight: window.innerHeight,
          childBottom: Math.round(Math.max(...childRects.map((childRect) => childRect.bottom))),
        };
      });
      assert(!emptyState.missing, "mobile vault dialog is missing an empty credential state", emptyState);
      assert(!emptyState.visible || emptyState.childBottom <= emptyState.viewportHeight - 6, "mobile vault empty state content is cut off in the first viewport", emptyState);
      assert(!emptyState.visible || emptyState.bottom <= emptyState.viewportHeight - 4, "mobile vault empty state card is cut off in the first viewport", emptyState);
    }
  }

  if (dialog.name === "vault" && viewportName === "desktop") {
    const saveButton = page.locator("[role='dialog'] .credential-form .save-credential").first();
    assert(await saveButton.count(), `${label} vault dialog is missing the save button`);
    const errorCountBeforeValidation = consoleErrors.length;
    await saveButton.click();
    await page.waitForTimeout(350);
    keepUnexpectedValidationConsoleErrors(consoleErrors, errorCountBeforeValidation);
    const formError = await page.evaluate(() => {
      const status = document.querySelector("[role='dialog'] .credential-form .vault-status")?.textContent || "";
      const toast = document.querySelector(".toast-message")?.textContent || "";
      return `${status} ${toast}`.replace(/\s+/g, " ").trim();
    });
    if (label.startsWith("zh-CN")) {
      assertLocalizedFeedback(label, formError, ["请填写交易所 API Key"], [/api key is required|save credential returned/i], "vault save");
    } else if (label.startsWith("en-US")) {
      assertLocalizedFeedback(label, formError, ["Enter the exchange API Key"], [/请填写|保存交易所密钥失败/], "vault save");
    }
    await closeToasts(page);
  }

  if (dialog.name === "ai-config" && viewportName === "desktop") {
    const nativeDialogs = [];
    const dialogHandler = async (nativeDialog) => {
      nativeDialogs.push({ type: nativeDialog.type(), message: nativeDialog.message() });
      await nativeDialog.dismiss().catch(() => null);
    };
    page.on("dialog", dialogHandler);
    try {
      await page.evaluate(() => {
        Object.defineProperty(navigator, "clipboard", {
          configurable: true,
          value: {
            writeText: async () => {
              throw new Error("forced clipboard denial");
            },
          },
        });
        document.execCommand = (command) => command === "copy";
      });

      const contextButton = page.locator("[role='dialog'] .subscription-assist-card .save-credential").first();
      assert(await contextButton.count(), `${label} AI config dialog is missing the copy context button`);
      await closeToasts(page);
      await contextButton.click();
      await page.waitForTimeout(180);
      assert(nativeDialogs.length === 0, `${label} copy AI context fell back to a native prompt`, nativeDialogs);
      assert(await contextButton.evaluate((element) => document.activeElement === element), `${label} copy AI context did not preserve keyboard focus on the trigger`);
      const contextToast = await toastText(page);
      assertLocalizedFeedback(label, contextToast, label.startsWith("zh-CN") ? ["AI 上下文已复制"] : ["AI context copied"], [/ready to copy|准备复制|prompt/i], "copy AI context");
      await closeToasts(page);

      const commandButton = page.locator("[role='dialog'] .provider-command-button").first();
      if (await commandButton.isVisible().catch(() => false)) {
        await commandButton.click();
        await page.waitForTimeout(180);
        assert(nativeDialogs.length === 0, `${label} copy AI command fell back to a native prompt`, nativeDialogs);
        assert(await commandButton.evaluate((element) => document.activeElement === element), `${label} copy AI command did not preserve keyboard focus on the trigger`);
        const commandToast = await toastText(page);
        assertLocalizedFeedback(label, commandToast, label.startsWith("zh-CN") ? ["命令已复制"] : ["Command copied"], [/ready to copy|准备复制|prompt/i], "copy AI command");
        await closeToasts(page);
      }
    } finally {
      page.off("dialog", dialogHandler);
    }
  }

  if (dialog.name === "live-guard" && viewportName === "desktop") {
    const unlockButton = page.locator("[role='dialog'] .guard-actions .save-credential").first();
    assert(await unlockButton.count(), `${label} live guard dialog is missing the unlock button`);
    const unlockErrorStart = consoleErrors.length;
    await unlockButton.click();
    await page.waitForTimeout(350);
    keepUnexpectedValidationConsoleErrors(consoleErrors, unlockErrorStart);
    const unlockFeedback = await page.evaluate(() => {
      const status = document.querySelector("[role='dialog'] .credential-form .vault-status")?.textContent || "";
      const toast = document.querySelector(".toast-message")?.textContent || "";
      return `${status} ${toast}`.replace(/\s+/g, " ").trim();
    });
    if (label.startsWith("zh-CN")) {
      assertLocalizedFeedback(label, unlockFeedback, ["请输入 ENABLE TESTNET LIVE"], [/unlock phrase must be|live guard update returned/i], "live guard unlock");
    } else if (label.startsWith("en-US")) {
      assertLocalizedFeedback(label, unlockFeedback, ["Type ENABLE TESTNET LIVE"], [/请输入|更新实盘护栏失败/], "live guard unlock");
    }
    await closeToasts(page);

    const executeButton = page.locator("[role='dialog'] .execute-live").first();
    assert(await executeButton.count(), `${label} live guard dialog is missing the execute button`);
    await executeButton.click();
    const executeFeedback = await toastText(page);
    if (label.startsWith("zh-CN")) {
      assertLocalizedFeedback(label, executeFeedback, ["实盘护栏已锁定"], [/Live Guard is locked|live execute returned/i], "blocked live execute");
    } else if (label.startsWith("en-US")) {
      assertLocalizedFeedback(label, executeFeedback, ["Live Guard is locked"], [/实盘护栏|执行失败/], "blocked live execute");
    }
    await closeToasts(page);

    const syncButton = page.locator("[role='dialog'] .sync-account").first();
    assert(await syncButton.count(), `${label} live guard dialog is missing the account sync button`);
    await syncButton.click();
    const syncFeedback = await toastText(page);
    if (label.startsWith("zh-CN")) {
      assertLocalizedFeedback(label, syncFeedback, ["请先选择已保存密钥", "需要密码短语"], [/Select a saved key first|Passphrase required|account sync returned/i], "blocked account sync");
    } else if (label.startsWith("en-US")) {
      assertLocalizedFeedback(label, syncFeedback, ["Select a saved key first", "Passphrase required"], [/请先选择|需要密码短语|账户快照/], "blocked account sync");
    }
    await closeToasts(page);
  }

  await page.keyboard.press("Escape");
  await assertDialogClosed("Escape");
  await assertFocusRestored("Escape");
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const report = {
    baseURL,
    locales,
    checkedAt: new Date().toISOString(),
    viewports: [],
  };

  try {
    for (const locale of locales) {
      for (const theme of themes) {
        for (const viewport of viewports) {
          const label = `${locale}-${theme}-${viewport.name}-${viewport.width}x${viewport.height}`;
          const { page, errors } = await newPage(browser, theme, viewport, locale);
          try {
            const layout = await layoutReport(page);
            assert(!layout.hasPageX, `${label} has document-level horizontal overflow`, layout);
            assert(layout.uncontainedOverflow.length === 0, `${label} has uncontained horizontal overflow`, layout.uncontainedOverflow);

            const nativeScrollbar = await assertNativeScrollbars(page, label);

            const controls = await assertControls(page, `${label} main screen`);
            const numberInputChrome = await assertNumberInputChrome(page, `${label} main screen`);
            await assertCriticalTextFits(page, label);
            const stateTextContrast = await assertStateTextContrast(page, label);
            const primaryHitTargets = await assertPrimaryHitTargets(page, label);
            const scrollableTabAffordance = await assertScrollableTabAffordance(page, label, viewport.name);
            const scrollableTabActivation = await assertScrollableTabActivation(page, label, viewport.name);
            const brandScale = await assertBrandScale(page, label);
            const runningMotion = await assertRunningMotion(page, label);
            const lightSegmentedChrome = await assertLightSegmentedChrome(page, label);
            const topBarLayer = await assertTopBarLayer(page, label);
            const leftRailStack = await assertLeftRailStack(page, label, viewport.name);
            const archiveDisclosure = await assertArchiveDisclosure(page, label, viewport.name);
            const chartFooter = await assertChartFooter(page, label);
            const workspaceVerticalStack = await assertWorkspaceVerticalStack(page, label, viewport.name);
            const eventLogFit = await assertEventLogFits(page, label);
            const headerSwitchers = await assertHeaderSwitchers(page, label, theme, locale);
            const topBarInteractions = await assertTopBarInteractions(page, label, viewport.name, theme);
            const bottomTableHorizontalScroll = await assertBottomTableHorizontalScroll(page, label, viewport.name);
            const paperResetDialog = await assertPaperResetDialog(page, label);
            const toastPlacement = await assertToastPlacement(page, label);
            const mainInteractions = await assertMainInteractions(page, label, viewport.name, theme);
            if (viewport.name === "mobile") {
              await assertMobilePrimaryContent(page, label);
            }
            if (viewport.name === "desktop") {
              await assertMenus(page, label);
            }
            if (viewport.name === "desktop" || viewport.name === "mobile") {
              const dialogs = [
                { name: "strategy", trigger: ".strategy-config-trigger" },
                { name: "ai-config", trigger: ".model-config-button" },
                { name: "vault", trigger: ".connection-link" },
                { name: "live-guard", trigger: ".guard-link" },
              ];
              for (const dialog of dialogs) {
                await assertDialog(page, dialog, viewport.name, label, errors);
              }
            }
            const screenshotPath = await screenshot(page, label);
            assert(errors.length === 0, `${label} emitted browser console errors`, errors);
            report.viewports.push({
              label,
              locale,
              controls,
              screenshotPath,
              layout: {
                hasPageX: layout.hasPageX,
                containedOverflow: {
                  count: layout.containedOverflow.length,
                  samples: layout.containedOverflow,
                },
              },
              nativeScrollbar,
              numberInputChrome,
              stateTextContrast,
              primaryHitTargets,
              scrollableTabAffordance,
              scrollableTabActivation,
              brandScale,
              runningMotion,
              lightSegmentedChrome,
              topBarLayer,
              leftRailStack,
              archiveDisclosure,
              chartFooter,
              workspaceVerticalStack,
              eventLogFit,
              headerSwitchers,
              topBarInteractions,
              bottomTableHorizontalScroll,
              paperResetDialog,
              toastPlacement,
              mainInteractions,
            });
            console.log(`ok ui ${label}`);
          } finally {
            await page.close();
          }
        }
      }
    }
  } finally {
    await browser.close();
  }

  const reportPath = path.join(outDir, "ui-quality-latest.json");
  await fs.writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`ok ui quality report ${reportPath}`);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : error);
  process.exit(1);
});
