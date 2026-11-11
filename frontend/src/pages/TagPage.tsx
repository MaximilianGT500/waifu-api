import { useParams } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import api, { httpStatus } from '../services/api';
import { ReviewStatus, Tag } from '../types';
import { MetaTags } from '../hooks/useMetaTags';
import EntityGallery from '../components/layout/EntityGallery';
import EntityHeader from '../components/EntityHeader';
import NotFound from './NotFound';

const SLUG = /^[a-z0-9-]{1,100}$/;

const TagPage = () => {
    const { slug } = useParams<{ slug: string }>();
    if (!slug || !SLUG.test(slug)) return <NotFound />;
    return <TagView slug={slug} />;
};

const TagView = ({ slug }: { slug: string }) => {
    const tag = useQuery({
        queryKey: ['tag', slug],
        staleTime: 60_000,
        retry: false,
        queryFn: async ({ signal }) => (await api.get<Tag>(`/tags/by-slug/${slug}`, { skipGlobalErrorHandler: true, signal })).data,
    });

    if (httpStatus(tag.error) === 404) return <NotFound />;

    const data = tag.data;
    const header = (
        <EntityHeader kind="Tag" backTo="/tags" backLabel="All tags" title={data?.name} pending={data?.reviewStatus === ReviewStatus.Pending}>
            {data?.description && <p className="page-lede">{data.description}</p>}
        </EntityHeader>
    );

    return (
        <>
            <MetaTags title={data ? `${data.name} images` : 'Tag'} description={data?.description || (data ? `Images tagged ${data.name}.` : undefined)} />
            <EntityGallery filterKey="includedTags" filterValue={slug} header={header} />
        </>
    );
};

export default TagPage;
