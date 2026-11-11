import { currentDesign } from './design';
import { effectsEnabled } from './motion';

const LIFETIME_MS = 1200;

export function sfx(text: string, anchor?: Element | null) {
    if (currentDesign() !== 'manga' || !effectsEnabled()) return;
    const rect = anchor?.getBoundingClientRect();
    const el = document.createElement('span');
    el.className = 'sfx';
    el.textContent = text;
    el.setAttribute('aria-hidden', 'true');
    el.style.left = `${rect ? rect.left + rect.width / 2 : window.innerWidth / 2}px`;
    el.style.top = `${rect ? rect.top : window.innerHeight / 2}px`;
    el.style.setProperty('--tilt', `${(Math.random() * 18 - 9).toFixed(1)}deg`);
    document.body.append(el);
    const remove = () => el.remove();
    el.addEventListener('animationend', remove, { once: true });
    window.setTimeout(remove, LIFETIME_MS);
}
