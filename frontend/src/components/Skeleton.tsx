import { HTMLAttributes } from 'react';

interface SkeletonProps extends HTMLAttributes<HTMLSpanElement> {
    className?: string;
}

const Skeleton = ({ className = '', ...props }: SkeletonProps) => (
    <span aria-hidden="true" className={`block animate-pulse rounded-lg bg-sleeve ${className}`} {...props} />
);

export default Skeleton;
