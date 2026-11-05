import { Info } from 'lucide-react';
import { ReviewStatus } from '../types';

export const AcceptSubmissionNote = ({ current, selected }: { current?: ReviewStatus; selected?: ReviewStatus }) => {
    if (current !== ReviewStatus.Pending || selected !== ReviewStatus.Accepted) return null;
    return (
        <div className="mt-2 flex items-start gap-2 rounded-2xl bg-warning/12 p-3 text-sm">
            <Info size={15} className="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
            <span className="text-foreground">Accepting this also approves any pending submission for it in the review queue. Edit suggestions aren't affected.</span>
        </div>
    );
};

export default AcceptSubmissionNote;
