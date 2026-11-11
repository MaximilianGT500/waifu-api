import { readLocal, writeLocal } from './storage';

const KEY = 'blur-nsfw';

export const readNsfwBlur = () => readLocal(KEY) === 'on';

export function setNsfwBlur(enabled: boolean) {
    writeLocal(KEY, enabled ? 'on' : 'off');
    const root = document.documentElement;
    if (enabled) root.dataset.blurNsfw = '';
    else delete root.dataset.blurNsfw;
}
