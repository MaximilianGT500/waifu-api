import { CSSProperties } from 'react';
import { Design } from '../utils/design';

interface DesignPreviewProps {
    design: Design;
    dark: boolean;
    accent: string;
}

const box = (style: CSSProperties): CSSProperties => ({ position: 'absolute', ...style });

const CollectorPreview = ({ dark, accent }: { dark: boolean; accent: string }) => {
    const canvas = dark ? '#09090b' : '#f4f4f5';
    const sheet = dark ? '#131316' : '#ffffff';
    const sleeve = dark ? '#0c0c0e' : '#ececef';
    const card = dark ? '#1c1c20' : '#ffffff';
    const tab = dark ? '#16161a' : '#e4e4e8';
    const ink = dark ? '#fafafa' : '#18181b';
    const shadow = dark ? '0 6px 14px -8px rgb(0 0 0 / 0.9)' : '0 6px 14px -8px rgb(9 9 11 / 0.35)';
    const tints = [`color-mix(in oklab, ${accent} 55%, #fde68a)`, `color-mix(in oklab, ${accent} 70%, #7dd3fc)`, `color-mix(in oklab, ${accent} 45%, #fda4af)`];
    return (
        <div style={{ position: 'absolute', inset: 0, background: canvas }}>
            {[0, 1, 2].map((i) => (
                <span key={i} style={box({ left: 6, top: 16 + i * 15, width: 26, height: 11, borderRadius: '6px 0 0 6px', background: i === 1 ? sheet : tab })} />
            ))}
            <div style={box({ left: 32, top: 8, right: 8, bottom: 0, background: sheet, borderRadius: '12px 12px 0 0', boxShadow: shadow })}>
                <span style={box({ left: 10, top: 9, width: '42%', height: 6, borderRadius: 4, background: ink, opacity: 0.85 })} />
                <span style={box({ left: 10, top: 19, width: '26%', height: 4, borderRadius: 4, background: ink, opacity: 0.3 })} />
                <div style={box({ left: 8, right: 8, top: 30, bottom: 6, background: sleeve, borderRadius: 9, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 5, padding: 5 })}>
                    {tints.map((tint, i) => (
                        <span key={i} style={{ position: 'relative', borderRadius: 6, background: card, padding: 2.5, boxShadow: shadow }}>
                            <span style={{ display: 'block', height: '100%', borderRadius: 4, background: `linear-gradient(160deg, ${tint}, color-mix(in oklab, ${tint} 60%, ${dark ? '#000' : '#fff'}))` }} />
                            {i === 1 && (
                                <span style={box({ inset: 2.5, borderRadius: 4, opacity: 0.55, mixBlendMode: 'color-dodge', background: 'repeating-linear-gradient(118deg, #7dd3fc 0%, #c4b5fd 9%, #f0abfc 18%, #fda4af 27%, #99f6e4 36%)' })} />
                            )}
                        </span>
                    ))}
                </div>
            </div>
        </div>
    );
};

const MangaPreview = ({ dark, accent }: { dark: boolean; accent: string }) => {
    const ink = dark ? '#e9e7e1' : '#141416';
    const paper = dark ? '#131315' : '#fffdf8';
    const canvas = dark ? '#0c0c0e' : '#efece4';
    const tone = dark ? 'rgb(233 231 225 / 0.13)' : 'rgb(20 20 22 / 0.2)';
    const shade = dark ? `color-mix(in oklab, ${accent} 64%, #000)` : ink;
    const line = (width: number, offset: number) => `0 0 0 ${width}px ${ink}, ${offset}px ${offset}px 0 ${width}px ${shade}`;
    return (
        <div style={{ position: 'absolute', inset: 0, background: canvas, backgroundImage: `radial-gradient(${tone} 1px, transparent 1.4px)`, backgroundSize: '6px 6px' }}>
            {[0, 1, 2].map((i) => (
                <span key={i} style={box({ left: 7, top: 16 + i * 15, width: 24, height: 11, borderRadius: '4px 0 0 4px', background: i === 1 ? ink : paper, boxShadow: `inset 0 0 0 1.5px ${ink}` })} />
            ))}
            <div style={box({ left: 33, top: 9, right: 11, bottom: -4, background: paper, borderRadius: 7, boxShadow: line(2, 4) })}>
                <span style={box({ left: 9, top: 8, width: '46%', height: 8, borderRadius: 2, background: ink, boxShadow: `2px 2px 0 color-mix(in oklab, ${accent} 60%, transparent)` })} />
                <span style={box({ right: 10, top: 6, width: 30, height: 13, borderRadius: 999, background: paper, boxShadow: `0 0 0 1.5px ${ink}` })} />
                <div style={box({ left: 8, right: 8, top: 29, bottom: 10, display: 'grid', gridTemplateColumns: '1.3fr 1fr 1fr', gap: 6 })}>
                    <span style={{ position: 'relative', borderRadius: 3, overflow: 'hidden', boxShadow: line(1.5, 2), background: `color-mix(in oklab, ${accent} 30%, ${paper})` }}>
                        <span style={box({ inset: 0, background: `repeating-conic-gradient(from 0deg at 50% 55%, ${ink} 0deg 1deg, transparent 1deg 7deg)`, opacity: 0.55, WebkitMaskImage: 'radial-gradient(circle at 50% 55%, transparent 0 30%, #000 75%)', maskImage: 'radial-gradient(circle at 50% 55%, transparent 0 30%, #000 75%)' })} />
                    </span>
                    <span style={{ borderRadius: 3, boxShadow: line(1.5, 2), background: paper, backgroundImage: `radial-gradient(${accent} 1.2px, transparent 1.6px)`, backgroundSize: '5px 5px' }} />
                    <span style={{ borderRadius: 3, boxShadow: line(1.5, 2), background: `linear-gradient(180deg, ${paper} 55%, color-mix(in oklab, ${ink} 85%, ${paper}) 55%)` }} />
                </div>
            </div>
        </div>
    );
};

const ClassicPreview = ({ dark, accent }: { dark: boolean; accent: string }) => {
    const page = dark ? '#09090b' : '#fafafa';
    const bar = dark ? '#111114' : '#ffffff';
    const line = dark ? '#27272a' : '#e4e4e7';
    const text = dark ? '#a1a1aa' : '#71717a';
    const thumb = dark ? '#27272a' : '#e4e4e7';
    return (
        <div style={{ position: 'absolute', inset: 0, background: page }}>
            <div style={box({ left: 0, right: 0, top: 0, height: 16, background: bar, borderBottom: `1px solid ${line}` })}>
                <span style={box({ left: 8, top: 5, width: 34, height: 5, borderRadius: 3, background: text })} />
                <span style={box({ right: 8, top: 4, width: 22, height: 8, borderRadius: 4, background: accent })} />
            </div>
            <div style={box({ left: 0, top: 16, bottom: 0, width: 34, background: bar, borderRight: `1px solid ${line}` })}>
                {[0, 1, 2, 3].map((i) => (
                    <span key={i} style={box({ left: 7, top: 9 + i * 10, width: 20, height: 4, borderRadius: 3, background: i === 0 ? accent : text, opacity: i === 0 ? 1 : 0.55 })} />
                ))}
            </div>
            <div style={box({ left: 42, right: 8, top: 24, bottom: 6, display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gridAutoRows: '1fr', gap: 5 })}>
                {Array.from({ length: 8 }, (_, i) => (
                    <span key={i} style={{ borderRadius: 3, background: i === 2 ? `color-mix(in oklab, ${accent} 40%, ${thumb})` : thumb }} />
                ))}
            </div>
        </div>
    );
};

const DesignPreview = ({ design, dark, accent }: DesignPreviewProps) => (
    <div className="relative aspect-[16/9] w-full overflow-hidden rounded-[0.6rem]" aria-hidden="true">
        {design === 'collector' && <CollectorPreview dark={dark} accent={accent} />}
        {design === 'manga' && <MangaPreview dark={dark} accent={accent} />}
        {design === 'classic' && <ClassicPreview dark={dark} accent={accent} />}
    </div>
);

export default DesignPreview;
