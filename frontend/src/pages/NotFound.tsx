import { Link } from 'react-router';

const NotFound = () => (
    <div className="flex min-h-[70vh] flex-col items-start justify-center gap-10 px-6 py-16 sm:flex-row sm:items-center sm:justify-center sm:gap-14">
        <div className="relative w-36 shrink-0 select-none sm:w-44" aria-hidden="true">
            <div className="card-back absolute inset-0 translate-x-3 rotate-[8deg] rounded-[1.25rem] shadow-card" />
            <div className="sleeve relative grid aspect-[5/7] -rotate-3 place-items-center rounded-[1.25rem] border-2 border-dashed border-input/60">
                <span className="text-gradient-waifu text-5xl font-black sm:text-6xl">404</span>
            </div>
        </div>
        <div className="max-w-md">
            <h1 className="text-3xl font-black">Page not found</h1>
            <p className="mt-3 text-muted-foreground">
                This page doesn't exist or was moved. The image or link may have been removed.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
                <Link to="/" className="btn btn-primary">Back to Home</Link>
                <Link to="/gallery" className="btn btn-secondary">Open the gallery</Link>
                <button type="button" onClick={() => window.history.back()} className="btn btn-ghost">Go Back</button>
            </div>
        </div>
    </div>
);

export default NotFound;
