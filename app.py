from flask import Flask, render_template, request, jsonify
from flask_sqlalchemy import SQLAlchemy
import json
import os

app = Flask(__name__)
app.config['SQLALCHEMY_DATABASE_URI'] = 'sqlite:///comments.db'
app.config['SQLALCHEMY_TRACK_MODIFICATIONS'] = False
db = SQLAlchemy(app)

class Comment(db.Model):
    id = db.Column(db.Integer, primary_key=True)
    pmid = db.Column(db.String(20), nullable=False)
    aspect = db.Column(db.String(10), nullable=False)
    model = db.Column(db.String(50), nullable=False)
    comment = db.Column(db.Text, nullable=False)

    def to_dict(self):
        return {
            'id': self.id,
            'pmid': self.pmid,
            'aspect': self.aspect,
            'model': self.model,
            'comment': self.comment
        }

def read_jsonl(file_path):
    data = []
    with open(file_path, 'r') as f:
        for line in f:
            data.append(json.loads(line.strip()))
    return data

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/api/comments/by-model', methods=['GET'])
def get_comments_by_model():
    """Get all comments grouped by model for the review page"""
    comments = Comment.query.all()
    
    # Group comments by model
    comments_by_model = {}
    for comment in comments:
        model = comment.model
        if model not in comments_by_model:
            comments_by_model[model] = []
        comments_by_model[model].append(comment.to_dict())
    
    # Sort comments within each model by pmid and aspect
    for model in comments_by_model:
        comments_by_model[model].sort(key=lambda x: (x['pmid'], x['aspect']))
    
    return jsonify(comments_by_model)

@app.route('/process', methods=['POST'])
def process():
    reference_file = request.files.get('reference')
    response_files = request.files.getlist('responses')
    
    if not reference_file or not response_files:
        return jsonify({'error': 'Missing files'}), 400

    # Read reference data and collect aspects per PMID
    reference_data = []
    pmid_aspects = {}  # Track aspects per PMID
    pmid_order = []   # Track PMID order
    aspect_order = [] # Track global aspect order to maintain original order
    
    for line in reference_file:
        ref = json.loads(line)
        reference_data.append(ref)
        
        # Track PMID order and aspects - ensure PMID is string
        if 'pmid' in ref:
            pmid = str(ref['pmid'])
            if pmid not in pmid_order:
                pmid_order.append(pmid)
            
            # Initialize aspects list for this PMID if not exists
            if pmid not in pmid_aspects:
                pmid_aspects[pmid] = []
            
            # Track aspect for this PMID while maintaining order
            if 'aspect' in ref:
                aspect = ref['aspect']
                # Add to global aspect order if not there
                if aspect not in aspect_order:
                    aspect_order.append(aspect)
                # Add to PMID-specific aspects if not there
                if aspect not in pmid_aspects[pmid]:
                    pmid_aspects[pmid].append(aspect)

    # Read response data and collect additional aspects per PMID
    response_data = {}
    for response_file in response_files:
        model_name = os.path.splitext(response_file.filename)[0]
        response_data[model_name] = []
        for line in response_file:
            resp = json.loads(line)
            # Convert PMID to string if present
            if 'pmid' in resp:
                resp['pmid'] = str(resp['pmid'])
                pmid = resp['pmid']
                
                # Track PMID order
                if pmid not in pmid_order:
                    pmid_order.append(pmid)
                
                # Initialize aspects list for this PMID if not exists
                if pmid not in pmid_aspects:
                    pmid_aspects[pmid] = []
                
                # Track aspect for this PMID while maintaining order
                if 'aspect' in resp:
                    aspect = resp['aspect']
                    # Add to global aspect order if not there
                    if aspect not in aspect_order:
                        aspect_order.append(aspect)
                    # Add to PMID-specific aspects if not there
                    if aspect not in pmid_aspects[pmid]:
                        pmid_aspects[pmid].append(aspect)
            
            response_data[model_name].append(resp)

    # Organize data by PMID while maintaining order
    organized_data = {'pmid_order': pmid_order}  # Include PMID order in response
    
    for pmid in pmid_order:
        # Sort PMID's aspects according to global aspect order
        pmid_aspect_list = sorted(pmid_aspects[pmid], key=lambda x: aspect_order.index(x))
        organized_data[pmid] = {
            'reference': {aspect: [] for aspect in pmid_aspect_list},
            'responses': {},
            'aspects': pmid_aspect_list  # Use the PMID-specific aspects list
        }
    
    # Add reference data
    for ref in reference_data:
        pmid = str(ref.get('pmid', ''))  # Convert to string
        if pmid in organized_data:
            aspect = ref.get('aspect', '')
            if aspect in organized_data[pmid]['reference']:
                organized_data[pmid]['reference'][aspect].append(ref)
    
    # Add response data
    for model, responses in response_data.items():
        for resp in responses:
            pmid = str(resp.get('pmid', ''))  # Convert to string
            if pmid in organized_data:
                if model not in organized_data[pmid]['responses']:
                    organized_data[pmid]['responses'][model] = {
                        aspect: [] for aspect in organized_data[pmid]['aspects']
                    }
                aspect = resp.get('aspect', '')
                if aspect in organized_data[pmid]['responses'][model]:
                    organized_data[pmid]['responses'][model][aspect].append(resp)

    return jsonify(organized_data)

@app.route('/comments', methods=['POST'])
def add_comment():
    data = request.json
    comment = Comment(
        pmid=data['pmid'],
        aspect=data['aspect'],
        model=data['model'],
        comment=data['comment']
    )
    db.session.add(comment)
    db.session.commit()
    return jsonify(comment.to_dict())

@app.route('/comments/<pmid>', methods=['GET'])
def get_comments(pmid):
    comments = Comment.query.filter_by(pmid=pmid).all()
    return jsonify([comment.to_dict() for comment in comments])

@app.route('/comments/<int:comment_id>', methods=['DELETE'])
def delete_comment(comment_id):
    comment = Comment.query.get_or_404(comment_id)
    db.session.delete(comment)
    db.session.commit()
    return '', 204

if __name__ == '__main__':
    with app.app_context():
        db.create_all()
    app.run(debug=True) 