import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

/**
 * Increment trailing `(N)` in a task title for duplicates.
 * "Buy milk (3)" → "Buy milk (4)", "Buy milk" → "Buy milk" (unchanged).
 */
export function incrementDuplicateTitle(content: string): string {
	return content.replace(/\((\d+)\)\s*$/, (_, n) => `(${Number(n) + 1})`);
}

/**
 * Strip leading prefix from a task title when copying.
 * Removes patterns like "Something: text", "Something - text", "Something – text".
 */
export function stripTaskPrefix(content: string): string {
	const stripped = content.replace(/^.+?(?::\s|\s[-–]\s)/, '');
	return stripped.trim() || content;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, "child"> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, "children"> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };
