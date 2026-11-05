import { HTMLAttributes } from 'react';

interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
    className?: string;
}

const Skeleton = ({ className = '', ...props }: SkeletonProps) => (
    <div className={`animate-pulse rounded-md bg-secondary ${className}`} {...props} />
);

export default Skeleton;
