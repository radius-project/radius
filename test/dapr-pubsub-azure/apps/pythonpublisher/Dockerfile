FROM python:3.7-slim
WORKDIR /app
ADD requirements.txt .
RUN pip install -r requirements.txt
COPY *.py /app/
CMD [ "python", "./pythonpublisher.py" ]
