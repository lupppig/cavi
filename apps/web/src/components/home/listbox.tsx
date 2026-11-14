"use client";

import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

export type ListboxOption = { value: string; label: string };

/**
 * A dropdown drawn by CAVI rather than the operating system, so its list
 * is readable on every platform and colour scheme. Follows the ARIA
 * select-only combobox pattern: arrows, Home/End, type-ahead, Enter or
 * Space to choose, Escape to close. A hidden input carries the value, so
 * it submits with its form like a native select.
 */
export function Listbox({
  name,
  label,
  placeholder,
  noneLabel,
  options,
  disabled,
  className = "",
}: {
  name: string;
  /** Accessible name. */
  label: string;
  placeholder: string;
  /** How the "no answer" choice reads in the list. */
  noneLabel: string;
  options: ListboxOption[];
  disabled?: boolean;
  className?: string;
}) {
  const id = useId();
  const [value, setValue] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const typed = useRef({ text: "", at: 0 });
  // The placeholder is a real choice: "no answer" (the field is optional).
  const all: ListboxOption[] = [{ value: "", label: noneLabel }, ...options];
  const selected = all.find((o) => o.value === value) ?? all[0];
  const shown = value ? selected.label : placeholder;

  useEffect(() => {
    if (!open) return;
    const away = (e: PointerEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("pointerdown", away);
    return () => document.removeEventListener("pointerdown", away);
  }, [open]);

  // Keep the highlighted option in view.
  useEffect(() => {
    if (open) document.getElementById(`${id}-opt-${active}`)?.scrollIntoView({ block: "nearest" });
  }, [open, active, id]);

  function openAt(i: number) {
    setActive(i);
    setOpen(true);
  }

  function choose(i: number) {
    setValue(all[i].value);
    setOpen(false);
    button.current?.focus();
  }

  function onKey(e: KeyboardEvent) {
    const last = all.length - 1;
    const current = all.indexOf(selected);
    if (!open) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
        e.preventDefault();
        openAt(e.key === "ArrowUp" ? Math.max(current - 1, 0) : current);
      }
      return;
    }
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setActive((a) => Math.min(a + 1, last));
        return;
      case "ArrowUp":
        e.preventDefault();
        setActive((a) => Math.max(a - 1, 0));
        return;
      case "Home":
        e.preventDefault();
        setActive(0);
        return;
      case "End":
        e.preventDefault();
        setActive(last);
        return;
      case "Enter":
      case " ":
        e.preventDefault();
        choose(active);
        return;
      case "Escape":
        e.preventDefault();
        setOpen(false);
        return;
      case "Tab":
        setOpen(false);
        return;
    }
    if (e.key.length === 1 && /\S/.test(e.key)) {
      const now = Date.now();
      const t = typed.current;
      t.text = now - t.at > 700 ? e.key.toLowerCase() : t.text + e.key.toLowerCase();
      t.at = now;
      const hit = all.findIndex((o, i) => i > 0 && o.label.toLowerCase().startsWith(t.text));
      if (hit >= 0) setActive(hit);
    }
  }

  return (
    <div ref={root} className={`relative ${className}`}>
      <input type="hidden" name={name} value={value} />
      <button
        ref={button}
        type="button"
        role="combobox"
        aria-label={label}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={`${id}-list`}
        aria-activedescendant={open ? `${id}-opt-${active}` : undefined}
        disabled={disabled}
        onClick={() => (open ? setOpen(false) : openAt(all.indexOf(selected)))}
        onKeyDown={onKey}
        className="flex min-h-13 w-full min-w-0 items-center justify-between gap-3 rounded-xl border border-[var(--line-strong)] bg-white px-4 text-left text-base font-medium text-[var(--fg)] focus:outline-none focus-visible:border-[var(--accent)] focus-visible:ring-3 focus-visible:ring-[var(--accent)]/20 disabled:opacity-50"
      >
        <span className={`truncate ${value ? "" : "text-[var(--muted)]"}`}>{shown}</span>
        <svg viewBox="0 0 12 8" className={`size-3 shrink-0 transition-transform ${open ? "rotate-180" : ""}`} aria-hidden="true">
          <path d="M1 1.5l5 5 5-5" fill="none" stroke="currentColor" strokeWidth="2" />
        </svg>
      </button>
      <ul
        id={`${id}-list`}
        role="listbox"
        aria-label={label}
        hidden={!open}
        className="absolute inset-x-0 top-full z-30 mt-1.5 max-h-[min(26rem,60vh)] overflow-y-auto overscroll-contain rounded-xl border border-[var(--line)] bg-white p-1 text-[var(--fg)] shadow-xl"
      >
        {all.map((o, i) => (
          <li
            key={o.value || "none"}
            id={`${id}-opt-${i}`}
            role="option"
            aria-selected={o.value === value}
            onPointerEnter={() => setActive(i)}
            onPointerDown={(e) => e.preventDefault()}
            onClick={() => choose(i)}
            className={`flex min-h-11 cursor-pointer items-center justify-between gap-3 rounded-lg px-3 text-base ${
              i === active ? "bg-[var(--accent-soft)] text-[var(--accent-text)]" : ""
            } ${i === 0 ? "italic" : "font-medium"}`}
          >
            {o.label}
            {o.value === value && <span aria-hidden="true">✓</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}
