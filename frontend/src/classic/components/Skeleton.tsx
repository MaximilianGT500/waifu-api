import { HTMLAttributes } from 'react';

interface SkeletonProps extends HTMLAttributes<HTMLSpanElement> {
    className?: string;
}

const Skeleton = ({ className = '', ...props }: SkeletonProps) => (
    <span aria-hidden="true" className={`block animate-pulse rounded-md bg-secondary ${className}`} {...props} />
);

export default Skeleton;
