import { useEffect } from 'react';
import { getEnv } from '../utils/env';

interface MetaTagsProps {
  title?: string;
  description?: string;
  image?: string;
  imageWidth?: number;
  imageHeight?: number;
  url?: string;
  type?: string;
  isNsfw?: boolean;
}

const defaultDescription = 'The API for your Waifu content. Access thousands of categorized anime illustrations via our robust REST API or browse freely.';

function setMeta(attribute: 'name' | 'property', key: string, value: string | null) {
  let element = document.head.querySelector<HTMLMetaElement>(`meta[${attribute}="${key}"]`);
  if (value === null) {
    element?.remove();
    return;
  }
  if (!element) {
    element = document.createElement('meta');
    element.setAttribute(attribute, key);
    document.head.appendChild(element);
  }
  element.setAttribute('content', value);
}

function applyMeta({ title, description, image, imageWidth, imageHeight, url, type = 'website', isNsfw = false }: MetaTagsProps) {
  const appTitle = getEnv('VITE_APP_TITLE') || 'Waifu API';
  const fullTitle = title ? `${title} - ${appTitle}` : appTitle;
  const desc = description || defaultDescription;
  const siteIcon = `${window.location.origin}/android-chrome-512x512.png?v=20261024-logo-v2`;
  const ogImage = image && !isNsfw ? image : siteIcon;

  document.title = fullTitle;
  setMeta('name', 'description', desc);
  setMeta('property', 'og:title', fullTitle);
  setMeta('property', 'og:description', desc);
  setMeta('property', 'og:image', ogImage);
  setMeta('property', 'og:image:width', imageWidth ? String(imageWidth) : null);
  setMeta('property', 'og:image:height', imageHeight ? String(imageHeight) : null);
  setMeta('property', 'og:type', type);
  setMeta('property', 'og:url', url || window.location.href);
  setMeta('name', 'twitter:card', image && !isNsfw ? 'summary_large_image' : 'summary');
  setMeta('name', 'twitter:title', fullTitle);
  setMeta('name', 'twitter:description', desc);
  setMeta('name', 'twitter:image', ogImage);
  setMeta('name', 'rating', isNsfw ? 'adult' : null);
}

export function MetaTags(props: MetaTagsProps) {
  const { title, description, image, imageWidth, imageHeight, url, type, isNsfw } = props;
  useEffect(() => {
    applyMeta({ title, description, image, imageWidth, imageHeight, url, type, isNsfw });
    return () => applyMeta({});
  }, [title, description, image, imageWidth, imageHeight, url, type, isNsfw]);
  return null;
}
