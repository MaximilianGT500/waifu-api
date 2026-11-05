import { Link } from 'react-router';

const NotFound = () => (
    <div className="flex min-h-[75vh] flex-col items-start justify-center px-6 py-16 sm:items-center sm:text-center">
        <p className="text-gradient-waifu text-[7rem] font-black leading-none tracking-[-0.04em] select-none sm:text-[9rem]" aria-hidden="true">404</p>
        <h1 className="mt-4 text-3xl font-extrabold tracking-[-0.02em]">Page not found</h1>
        <p className="mt-3 max-w-md text-muted-foreground">
            This page doesn't exist or was moved. The image or link may have been removed.
        </p>
        <div className="mt-8 flex flex-wrap gap-3">
            <Link to="/" className="btn btn-primary">Back to Home</Link>
            <Link to="/gallery" className="btn btn-secondary">Open the gallery</Link>
            <button type="button" onClick={() => window.history.back()} className="btn btn-ghost">Go Back</button>
        </div>
    </div>
);

export default NotFound;
